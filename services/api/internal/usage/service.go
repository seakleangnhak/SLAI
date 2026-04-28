package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/apikeys"
	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/omniroute"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var (
	ErrInvalidUsageEvent  = errors.New("invalid usage event")
	ErrSyncNotImplemented = errors.New("omniroute usage sync is not implemented by the configured client")
)

type Service struct {
	pool         *pgxpool.Pool
	omniRoute    omniroute.Client
	omniRouteCfg config.OmniRouteConfig
	logger       *slog.Logger
}

func NewService(pool *pgxpool.Pool, omniRoute omniroute.Client, omniRouteCfg config.OmniRouteConfig, logger *slog.Logger) Service {
	if logger == nil {
		logger = slog.Default()
	}
	return Service{pool: pool, omniRoute: omniRoute, omniRouteCfg: omniRouteCfg, logger: logger}
}

func (s Service) IngestMockEvent(ctx context.Context, input IngestInput) (IngestResult, error) {
	input.ExternalSource = ExternalSourceMock
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now().UTC()
	}
	return s.IngestEvent(ctx, input)
}

func (s Service) IngestEvent(ctx context.Context, input IngestInput) (IngestResult, error) {
	if err := validateInput(input); err != nil {
		return IngestResult{}, err
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Now().UTC()
	}

	var result IngestResult
	err := platformdb.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		existing, found, err := findUsageEvent(ctx, tx, input.ExternalSource, input.ExternalEventID)
		if err != nil {
			return err
		}
		if found {
			result = IngestResult{Event: &existing, Status: StatusDuplicate, Duplicate: true}
			return nil
		}

		key, found, err := resolveAPIKey(ctx, tx, input)
		if err != nil {
			return err
		}
		if !found {
			result = IngestResult{Status: StatusIgnored, Ignored: true}
			return nil
		}

		costUnits, pricingMetadata, err := s.calculateCost(ctx, tx, input)
		if err != nil {
			return err
		}

		event, inserted, err := insertUsageEvent(ctx, tx, key, input, costUnits, StatusBilled)
		if err != nil {
			return err
		}
		if !inserted {
			existing, found, err := findUsageEvent(ctx, tx, input.ExternalSource, input.ExternalEventID)
			if err != nil {
				return err
			}
			if found {
				result = IngestResult{Event: &existing, Status: StatusDuplicate, Duplicate: true}
				return nil
			}
			return errors.New("usage event insert conflicted but existing event was not found")
		}

		suspendedKey := false
		if costUnits > 0 {
			eventID := event.ID
			idempotencyKey := usageIdempotencyKey(input.ExternalSource, input.ExternalEventID)
			reason := "OmniRoute usage billing"
			metadata := map[string]any{
				"externalSource":  input.ExternalSource,
				"externalEventId": input.ExternalEventID,
			}
			for key, value := range pricingMetadata {
				metadata[key] = value
			}

			_, balance, err := ledger.NewService(tx).Mutate(ctx, ledger.Mutation{
				UserID:         key.UserID,
				Type:           ledger.TypeUsageDebit,
				Source:         "omniroute",
				DeltaUnits:     -costUnits,
				UsageEventID:   &eventID,
				IdempotencyKey: &idempotencyKey,
				Reason:         &reason,
				Metadata:       metadata,
			})
			if err != nil {
				return err
			}
			if balance.AvailableUnits <= 0 {
				suspended, err := s.suspendResolvedKey(ctx, tx, key)
				if err != nil {
					return err
				}
				suspendedKey = suspended
			}
		}

		result = IngestResult{Event: &event, Status: StatusBilled, SuspendedKey: suspendedKey}
		return nil
	})
	if err != nil {
		return IngestResult{}, err
	}

	return result, nil
}

func (s Service) SyncOmniRoute(ctx context.Context, limit int) (SyncResult, error) {
	if s.omniRoute == nil {
		return SyncResult{}, ErrSyncNotImplemented
	}
	if limit <= 0 {
		limit = s.omniRouteCfg.CallLogLimit
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	source := ExternalSourceOmniRouteCallLogs
	if s.omniRouteCfg.UsageSyncMode == "usage_history" {
		source = ExternalSourceOmniRouteUsageHistory
	}

	state, err := s.getSyncState(ctx, source)
	if err != nil {
		return SyncResult{}, err
	}

	if s.omniRouteCfg.UsageSyncMode == "usage_history" {
		return s.syncUsageHistory(ctx, state, limit)
	}
	return s.syncCallLogs(ctx, state, limit)
}

func (s Service) syncCallLogs(ctx context.Context, state syncState, limit int) (SyncResult, error) {
	logs, err := s.omniRoute.FetchCallLogs(ctx, state.LastSeenTimestamp, limit)
	if errors.Is(err, omniroute.ErrNotImplemented) {
		return SyncResult{}, ErrSyncNotImplemented
	}
	if err != nil {
		return SyncResult{}, err
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].OccurredAt.Before(logs[j].OccurredAt) })

	result := SyncResult{Fetched: len(logs)}
	for _, log := range logs {
		input := IngestInput{
			ExternalSource:  ExternalSourceOmniRouteCallLogs,
			ExternalEventID: log.ExternalID,
			OmniRouteKeyID:  nullableString(log.APIKeyID),
			Model:           nullableString(log.Model),
			Provider:        nullableString(log.Provider),
			InputTokens:     log.InputTokens,
			OutputTokens:    log.OutputTokens,
			OccurredAt:      log.OccurredAt,
			Raw:             log.Raw,
		}
		ingested, err := s.IngestEvent(ctx, input)
		if err != nil {
			result.Failed++
			return result, err
		}
		result.add(ingested)
		if err := s.updateSyncState(ctx, ExternalSourceOmniRouteCallLogs, log.OccurredAt, log.ExternalID); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s Service) syncUsageHistory(ctx context.Context, state syncState, limit int) (SyncResult, error) {
	records, err := s.omniRoute.FetchUsageHistory(ctx, state.LastSeenTimestamp, limit)
	if errors.Is(err, omniroute.ErrNotImplemented) {
		return SyncResult{}, ErrSyncNotImplemented
	}
	if err != nil {
		return SyncResult{}, err
	}
	sort.Slice(records, func(i, j int) bool { return records[i].OccurredAt.Before(records[j].OccurredAt) })

	result := SyncResult{Fetched: len(records)}
	for _, record := range records {
		costUnits := record.CostUnits
		input := IngestInput{
			ExternalSource:    ExternalSourceOmniRouteUsageHistory,
			ExternalEventID:   record.ExternalID,
			OmniRouteKeyID:    nullableString(record.APIKeyID),
			Model:             nullableString(record.Model),
			Provider:          nullableString(record.Provider),
			OccurredAt:        record.OccurredAt,
			Raw:               record.Raw,
			CostUnitsOverride: &costUnits,
		}
		ingested, err := s.IngestEvent(ctx, input)
		if err != nil {
			result.Failed++
			return result, err
		}
		result.add(ingested)
		if err := s.updateSyncState(ctx, ExternalSourceOmniRouteUsageHistory, record.OccurredAt, record.ExternalID); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s Service) ListEvents(ctx context.Context, filter ListFilter) ([]Event, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	query := `
		SELECT id::text, user_id::text, api_key_id::text, external_source, external_event_id,
		       omniroute_key_id, model, provider, input_tokens, output_tokens, total_tokens,
		       cost_units, status, occurred_at, raw_json, created_at
		FROM usage_events
		WHERE 1 = 1
	`
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		query += fmt.Sprintf(" AND %s = $%d", clause, len(args))
	}
	if filter.UserID != nil && *filter.UserID != "" {
		add("user_id", *filter.UserID)
	}
	if filter.APIKeyID != nil && *filter.APIKeyID != "" {
		add("api_key_id", *filter.APIKeyID)
	}
	if filter.Model != nil && *filter.Model != "" {
		add("model", *filter.Model)
	}
	if filter.Provider != nil && *filter.Provider != "" {
		add("provider", *filter.Provider)
	}
	if filter.Status != nil && *filter.Status != "" {
		add("status", *filter.Status)
	}
	if filter.StartTime != nil {
		args = append(args, *filter.StartTime)
		query += fmt.Sprintf(" AND occurred_at >= $%d", len(args))
	}
	if filter.EndTime != nil {
		args = append(args, *filter.EndTime)
		query += fmt.Sprintf(" AND occurred_at <= $%d", len(args))
	}
	args = append(args, limit, filter.Offset)
	query += fmt.Sprintf(" ORDER BY occurred_at DESC, created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args))

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list usage events: %w", err)
	}
	defer rows.Close()

	events := []Event{}
	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func (s Service) calculateCost(ctx context.Context, tx pgx.Tx, input IngestInput) (int64, map[string]any, error) {
	if input.CostUnitsOverride != nil {
		if *input.CostUnitsOverride < 0 {
			return 0, nil, ErrInvalidUsageEvent
		}
		return *input.CostUnitsOverride, map[string]any{"costSource": "omniroute_usage_history"}, nil
	}

	costUnits, pricingRule, err := NewPricingService(tx).CalculateCost(ctx, input.Provider, input.Model, input.InputTokens, input.OutputTokens)
	if err != nil {
		return 0, nil, err
	}
	return costUnits, map[string]any{"pricingRuleId": pricingRule.ID}, nil
}

func (s Service) suspendResolvedKey(ctx context.Context, tx pgx.Tx, key resolvedAPIKey) (bool, error) {
	if key.Status != apikeys.StatusActive {
		return false, nil
	}

	if s.omniRouteCfg.Enabled && key.OmniRouteKeyID != nil && *key.OmniRouteKeyID != "" {
		if s.omniRoute == nil {
			return false, errors.New("omniroute client is required when OMNIROUTE_ENABLED=true")
		}
		isActive := false
		if err := s.omniRoute.UpdateAPIKey(ctx, *key.OmniRouteKeyID, omniroute.UpdateAPIKeyPayload{IsActive: &isActive}); err != nil {
			return false, fmt.Errorf("suspend omniroute api key after usage billing: %w", err)
		}
	}

	_, err := tx.Exec(ctx, `
		UPDATE api_keys
		SET status = $2
		WHERE id = $1 AND status = $3
	`, key.ID, apikeys.StatusSuspended, apikeys.StatusActive)
	if err != nil {
		return false, fmt.Errorf("suspend api key after usage billing: %w", err)
	}

	s.logger.Info("api key suspended after usage billing", "user_id", key.UserID, "key_id", key.ID)
	return true, nil
}

func (r *SyncResult) add(result IngestResult) {
	if result.Duplicate {
		r.Duplicate++
		return
	}
	if result.Ignored {
		r.Ignored++
		return
	}
	if result.Status == StatusBilled {
		r.Billed++
	}
	if result.SuspendedKey {
		r.SuspendedKeys++
	}
}

func validateInput(input IngestInput) error {
	if strings.TrimSpace(input.ExternalSource) == "" || strings.TrimSpace(input.ExternalEventID) == "" {
		return ErrInvalidUsageEvent
	}
	if input.InputTokens < 0 || input.OutputTokens < 0 {
		return ErrInvalidUsageEvent
	}
	return nil
}

func resolveAPIKey(ctx context.Context, tx pgx.Tx, input IngestInput) (resolvedAPIKey, bool, error) {
	if input.OmniRouteKeyID != nil && *input.OmniRouteKeyID != "" {
		key, found, err := findAPIKey(ctx, tx, `omniroute_key_id = $1`, *input.OmniRouteKeyID)
		if err != nil || found {
			return key, found, err
		}
	}
	if input.APIKeyID != nil && *input.APIKeyID != "" {
		return findAPIKey(ctx, tx, `id = $1`, *input.APIKeyID)
	}
	return resolvedAPIKey{}, false, nil
}

type resolvedAPIKey struct {
	ID             string
	UserID         string
	OmniRouteKeyID *string
	Status         string
}

func findAPIKey(ctx context.Context, tx pgx.Tx, where string, value any) (resolvedAPIKey, bool, error) {
	var key resolvedAPIKey
	err := tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, omniroute_key_id, status
		FROM api_keys
		WHERE `+where+`
		LIMIT 1
	`, value).Scan(&key.ID, &key.UserID, &key.OmniRouteKeyID, &key.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return resolvedAPIKey{}, false, nil
	}
	if err != nil {
		return resolvedAPIKey{}, false, err
	}
	return key, true, nil
}

func insertUsageEvent(ctx context.Context, tx pgx.Tx, key resolvedAPIKey, input IngestInput, costUnits int64, status string) (Event, bool, error) {
	rawJSON, err := json.Marshal(input.Raw)
	if err != nil {
		return Event{}, false, err
	}
	if input.Raw == nil {
		rawJSON = nil
	}
	totalTokens := input.InputTokens + input.OutputTokens

	event, err := scanEvent(tx.QueryRow(ctx, `
		INSERT INTO usage_events (
			user_id, api_key_id, external_source, external_event_id, omniroute_key_id,
			model, provider, input_tokens, output_tokens, total_tokens, cost_units,
			status, occurred_at, raw_json
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (external_source, external_event_id) DO NOTHING
		RETURNING id::text, user_id::text, api_key_id::text, external_source, external_event_id,
		          omniroute_key_id, model, provider, input_tokens, output_tokens, total_tokens,
		          cost_units, status, occurred_at, raw_json, created_at
	`,
		key.UserID,
		key.ID,
		input.ExternalSource,
		input.ExternalEventID,
		key.OmniRouteKeyID,
		input.Model,
		input.Provider,
		input.InputTokens,
		input.OutputTokens,
		totalTokens,
		costUnits,
		status,
		input.OccurredAt,
		rawJSON,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	return event, true, nil
}

func findUsageEvent(ctx context.Context, tx pgx.Tx, source string, externalID string) (Event, bool, error) {
	event, err := scanEvent(tx.QueryRow(ctx, `
		SELECT id::text, user_id::text, api_key_id::text, external_source, external_event_id,
		       omniroute_key_id, model, provider, input_tokens, output_tokens, total_tokens,
		       cost_units, status, occurred_at, raw_json, created_at
		FROM usage_events
		WHERE external_source = $1 AND external_event_id = $2
	`, source, externalID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Event{}, false, nil
	}
	if err != nil {
		return Event{}, false, err
	}
	return event, true, nil
}

func scanEvent(row pgx.Row) (Event, error) {
	var event Event
	var raw []byte
	err := row.Scan(
		&event.ID,
		&event.UserID,
		&event.APIKeyID,
		&event.ExternalSource,
		&event.ExternalEventID,
		&event.OmniRouteKeyID,
		&event.Model,
		&event.Provider,
		&event.InputTokens,
		&event.OutputTokens,
		&event.TotalTokens,
		&event.CostUnits,
		&event.Status,
		&event.OccurredAt,
		&raw,
		&event.CreatedAt,
	)
	if err != nil {
		return Event{}, err
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &event.RawJSON)
	}
	return event, nil
}

type syncState struct {
	Source             string
	LastSeenTimestamp  *time.Time
	LastSeenExternalID *string
}

func (s Service) getSyncState(ctx context.Context, source string) (syncState, error) {
	var state syncState
	err := s.pool.QueryRow(ctx, `
		INSERT INTO omniroute_sync_state (source)
		VALUES ($1)
		ON CONFLICT (source) DO UPDATE SET source = EXCLUDED.source
		RETURNING source, last_seen_timestamp, last_seen_external_id
	`, source).Scan(&state.Source, &state.LastSeenTimestamp, &state.LastSeenExternalID)
	if err != nil {
		return syncState{}, err
	}
	return state, nil
}

func (s Service) updateSyncState(ctx context.Context, source string, timestamp time.Time, externalID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO omniroute_sync_state (source, last_seen_timestamp, last_seen_external_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (source) DO UPDATE
		SET last_seen_timestamp = EXCLUDED.last_seen_timestamp,
		    last_seen_external_id = EXCLUDED.last_seen_external_id
	`, source, timestamp, externalID)
	return err
}

func usageIdempotencyKey(source string, externalID string) string {
	return "usage:" + source + ":" + externalID
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

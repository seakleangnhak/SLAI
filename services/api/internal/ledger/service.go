package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var (
	ErrInvalidMutation     = errors.New("invalid ledger mutation")
	ErrMissingReason       = errors.New("admin adjustment requires reason")
	ErrIdempotencyConflict = errors.New("idempotency key belongs to a different ledger mutation")
)

type Service struct {
	db platformdb.Executor
}

func NewService(db platformdb.Executor) Service {
	return Service{db: db}
}

func (s Service) EnsureBalance(ctx context.Context, userID string) error {
	tx, ok := s.db.(pgx.Tx)
	if !ok {
		return errors.New("EnsureBalance requires a transaction")
	}
	if err := platformdb.EnableLedgerMutation(ctx, tx); err != nil {
		return err
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO credit_balances (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("ensure balance: %w", err)
	}
	return nil
}

func (s Service) Mutate(ctx context.Context, mutation Mutation) (Entry, Balance, error) {
	if err := validateMutation(mutation); err != nil {
		return Entry{}, Balance{}, err
	}

	tx, ok := s.db.(pgx.Tx)
	if !ok {
		return Entry{}, Balance{}, errors.New("ledger mutation requires a transaction")
	}

	if mutation.IdempotencyKey != nil && *mutation.IdempotencyKey != "" {
		entry, found, err := s.GetByIdempotencyKey(ctx, *mutation.IdempotencyKey)
		if err != nil {
			return Entry{}, Balance{}, err
		}
		if found {
			if !entryMatchesMutation(entry, mutation) {
				return Entry{}, Balance{}, ErrIdempotencyConflict
			}
			balance, err := s.GetBalance(ctx, mutation.UserID)
			if err != nil {
				return Entry{}, Balance{}, err
			}
			return entry, balance, nil
		}
	}

	if err := platformdb.EnableLedgerMutation(ctx, tx); err != nil {
		return Entry{}, Balance{}, err
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO credit_balances (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, mutation.UserID)
	if err != nil {
		return Entry{}, Balance{}, fmt.Errorf("ensure balance before mutation: %w", err)
	}

	var current Balance
	err = tx.QueryRow(ctx, `
		SELECT user_id::text, available_units, lifetime_purchased_units, lifetime_used_units, version, updated_at
		FROM credit_balances
		WHERE user_id = $1
		FOR UPDATE
	`, mutation.UserID).Scan(
		&current.UserID,
		&current.AvailableUnits,
		&current.LifetimePurchasedUnits,
		&current.LifetimeUsedUnits,
		&current.Version,
		&current.UpdatedAt,
	)
	if err != nil {
		return Entry{}, Balance{}, fmt.Errorf("load balance for update: %w", err)
	}

	newAvailable := current.AvailableUnits + mutation.DeltaUnits
	newLifetimePurchased := current.LifetimePurchasedUnits
	newLifetimeUsed := current.LifetimeUsedUnits

	switch mutation.Type {
	case TypePaymentCredit, TypeBonusCredit:
		if mutation.DeltaUnits > 0 {
			newLifetimePurchased += mutation.DeltaUnits
		}
	case TypeUsageDebit:
		if mutation.DeltaUnits < 0 {
			newLifetimeUsed += -mutation.DeltaUnits
		}
	}

	var balance Balance
	err = tx.QueryRow(ctx, `
		UPDATE credit_balances
		SET available_units = $2,
		    lifetime_purchased_units = $3,
		    lifetime_used_units = $4,
		    version = version + 1
		WHERE user_id = $1
		RETURNING user_id::text, available_units, lifetime_purchased_units, lifetime_used_units, version, updated_at
	`, mutation.UserID, newAvailable, newLifetimePurchased, newLifetimeUsed).Scan(
		&balance.UserID,
		&balance.AvailableUnits,
		&balance.LifetimePurchasedUnits,
		&balance.LifetimeUsedUnits,
		&balance.Version,
		&balance.UpdatedAt,
	)
	if err != nil {
		return Entry{}, Balance{}, fmt.Errorf("update balance: %w", err)
	}

	metadata, err := json.Marshal(mutation.Metadata)
	if err != nil {
		return Entry{}, Balance{}, fmt.Errorf("marshal ledger metadata: %w", err)
	}
	if mutation.Metadata == nil {
		metadata = nil
	}

	var entry Entry
	var metadataBytes []byte
	err = tx.QueryRow(ctx, `
		INSERT INTO credit_ledger_entries (
			user_id, type, source, delta_units, balance_after_units,
			payment_id, usage_event_id, admin_id, idempotency_key, reason, metadata
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id::text, user_id::text, type, source, delta_units, balance_after_units,
		          payment_id::text, usage_event_id::text, admin_id::text, idempotency_key, reason, metadata, created_at
	`,
		mutation.UserID,
		mutation.Type,
		mutation.Source,
		mutation.DeltaUnits,
		balance.AvailableUnits,
		mutation.PaymentID,
		mutation.UsageEventID,
		mutation.AdminID,
		mutation.IdempotencyKey,
		mutation.Reason,
		metadata,
	).Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Type,
		&entry.Source,
		&entry.DeltaUnits,
		&entry.BalanceAfterUnits,
		&entry.PaymentID,
		&entry.UsageEventID,
		&entry.AdminID,
		&entry.IdempotencyKey,
		&entry.Reason,
		&metadataBytes,
		&entry.CreatedAt,
	)
	if err != nil {
		return Entry{}, Balance{}, fmt.Errorf("insert ledger entry: %w", err)
	}
	entry.Metadata = decodeMetadata(metadataBytes)

	return entry, balance, nil
}

func (s Service) GetBalance(ctx context.Context, userID string) (Balance, error) {
	var balance Balance
	err := s.db.QueryRow(ctx, `
		SELECT user_id::text, available_units, lifetime_purchased_units, lifetime_used_units, version, updated_at
		FROM credit_balances
		WHERE user_id = $1
	`, userID).Scan(
		&balance.UserID,
		&balance.AvailableUnits,
		&balance.LifetimePurchasedUnits,
		&balance.LifetimeUsedUnits,
		&balance.Version,
		&balance.UpdatedAt,
	)
	if err != nil {
		return Balance{}, fmt.Errorf("get balance: %w", err)
	}
	return balance, nil
}

func (s Service) ListEntries(ctx context.Context, userID string, limit int) ([]Entry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := s.db.Query(ctx, `
		SELECT id::text, user_id::text, type, source, delta_units, balance_after_units,
		       payment_id::text, usage_event_id::text, admin_id::text, idempotency_key, reason, metadata, created_at
		FROM credit_ledger_entries
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list ledger entries: %w", err)
	}
	defer rows.Close()

	entries := []Entry{}
	for rows.Next() {
		var entry Entry
		var metadataBytes []byte
		if err := rows.Scan(
			&entry.ID,
			&entry.UserID,
			&entry.Type,
			&entry.Source,
			&entry.DeltaUnits,
			&entry.BalanceAfterUnits,
			&entry.PaymentID,
			&entry.UsageEventID,
			&entry.AdminID,
			&entry.IdempotencyKey,
			&entry.Reason,
			&metadataBytes,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}
		entry.Metadata = decodeMetadata(metadataBytes)
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger entries: %w", err)
	}

	return entries, nil
}

func (s Service) GetByIdempotencyKey(ctx context.Context, key string) (Entry, bool, error) {
	var entry Entry
	var metadataBytes []byte
	err := s.db.QueryRow(ctx, `
		SELECT id::text, user_id::text, type, source, delta_units, balance_after_units,
		       payment_id::text, usage_event_id::text, admin_id::text, idempotency_key, reason, metadata, created_at
		FROM credit_ledger_entries
		WHERE idempotency_key = $1
	`, key).Scan(
		&entry.ID,
		&entry.UserID,
		&entry.Type,
		&entry.Source,
		&entry.DeltaUnits,
		&entry.BalanceAfterUnits,
		&entry.PaymentID,
		&entry.UsageEventID,
		&entry.AdminID,
		&entry.IdempotencyKey,
		&entry.Reason,
		&metadataBytes,
		&entry.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("find ledger entry by idempotency key: %w", err)
	}
	entry.Metadata = decodeMetadata(metadataBytes)
	return entry, true, nil
}

func entryMatchesMutation(entry Entry, mutation Mutation) bool {
	return entry.UserID == mutation.UserID &&
		entry.Type == mutation.Type &&
		entry.Source == mutation.Source &&
		entry.DeltaUnits == mutation.DeltaUnits &&
		optionalStringEqual(entry.PaymentID, mutation.PaymentID) &&
		optionalStringEqual(entry.UsageEventID, mutation.UsageEventID) &&
		optionalStringEqual(entry.AdminID, mutation.AdminID) &&
		optionalStringEqual(entry.Reason, mutation.Reason)
}

func optionalStringEqual(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validateMutation(mutation Mutation) error {
	if mutation.UserID == "" || mutation.Type == "" || mutation.Source == "" || mutation.DeltaUnits == 0 {
		return ErrInvalidMutation
	}

	switch mutation.Type {
	case TypePaymentCredit, TypeAdminAdjustmentCredit, TypeBonusCredit:
		if mutation.DeltaUnits <= 0 {
			return ErrInvalidMutation
		}
	case TypeUsageDebit, TypeAdminAdjustmentDebit, TypeRefundDebit:
		if mutation.DeltaUnits >= 0 {
			return ErrInvalidMutation
		}
	default:
		return ErrInvalidMutation
	}

	if (mutation.Type == TypeAdminAdjustmentCredit || mutation.Type == TypeAdminAdjustmentDebit) && (mutation.Reason == nil || *mutation.Reason == "") {
		return ErrMissingReason
	}

	return nil
}

func decodeMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil
	}
	return metadata
}

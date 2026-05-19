package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

var ErrInvalidAdminUserFilter = errors.New("invalid admin user filter")

type AdminRepository struct {
	db platformdb.Executor
}

func NewAdminRepository(db platformdb.Executor) AdminRepository {
	return AdminRepository{db: db}
}

func (r AdminRepository) List(ctx context.Context, filter AdminListFilter) (AdminUserListResult, error) {
	filter = normalizeAdminListFilter(filter)
	if err := validateAdminListFilter(filter); err != nil {
		return AdminUserListResult{}, err
	}

	where, args := adminUserWhere(filter)
	countQuery := `SELECT count(*) FROM users u ` + where

	var total int64
	if err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return AdminUserListResult{}, fmt.Errorf("count admin users: %w", err)
	}

	args = append(args, filter.Limit, filter.Offset)
	query := `
		SELECT u.id::text, u.email, u.role, u.status, u.auth_provider,
		       COALESCE(cb.available_units, 0),
		       COALESCE(cb.lifetime_purchased_units, 0),
		       COALESCE(cb.lifetime_used_units, 0),
		       ak.status, ak.key_prefix,
		       u.created_at, u.updated_at
		FROM users u
		LEFT JOIN credit_balances cb ON cb.user_id = u.id
		LEFT JOIN LATERAL (
			SELECT status, key_prefix
			FROM api_keys
			WHERE user_id = u.id
			ORDER BY created_at DESC, id DESC
			LIMIT 1
		) ak ON true
		` + where + `
		ORDER BY u.created_at DESC, u.id DESC
		LIMIT $` + fmt.Sprint(len(args)-1) + ` OFFSET $` + fmt.Sprint(len(args))

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return AdminUserListResult{}, fmt.Errorf("list admin users: %w", err)
	}
	defer rows.Close()

	items := []AdminUserListItem{}
	for rows.Next() {
		var item AdminUserListItem
		if err := rows.Scan(
			&item.ID,
			&item.Email,
			&item.Role,
			&item.Status,
			&item.AuthProvider,
			&item.BalanceUnits,
			&item.LifetimePurchasedUnits,
			&item.LifetimeUsedUnits,
			&item.APIKeyStatus,
			&item.APIKeyPrefix,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return AdminUserListResult{}, fmt.Errorf("scan admin user list item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return AdminUserListResult{}, fmt.Errorf("iterate admin users: %w", err)
	}

	return AdminUserListResult{Items: items, Limit: filter.Limit, Offset: filter.Offset, Total: total}, nil
}

func (r AdminRepository) GetDetail(ctx context.Context, id string) (AdminUserDetail, error) {
	var detail AdminUserDetail
	err := r.db.QueryRow(ctx, `
		SELECT u.id::text, u.email, u.role, u.status, u.auth_provider,
		       COALESCE(cb.available_units, 0),
		       COALESCE(cb.lifetime_purchased_units, 0),
		       COALESCE(cb.lifetime_used_units, 0),
		       COALESCE(cb.version, 0),
		       COALESCE(cb.updated_at, u.updated_at),
		       u.created_at, u.updated_at
		FROM users u
		LEFT JOIN credit_balances cb ON cb.user_id = u.id
		WHERE u.id = $1
	`, id).Scan(
		&detail.ID,
		&detail.Email,
		&detail.Role,
		&detail.Status,
		&detail.AuthProvider,
		&detail.Balance.AvailableUnits,
		&detail.Balance.LifetimePurchasedUnits,
		&detail.Balance.LifetimeUsedUnits,
		&detail.Balance.Version,
		&detail.Balance.UpdatedAt,
		&detail.CreatedAt,
		&detail.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminUserDetail{}, ErrNotFound
	}
	if err != nil {
		return AdminUserDetail{}, fmt.Errorf("get admin user detail: %w", err)
	}

	apiKey, found, err := r.getLatestAPIKey(ctx, id)
	if err != nil {
		return AdminUserDetail{}, err
	}
	if found {
		detail.APIKey = &apiKey
	}

	usageRows, err := r.listRecentUsage(ctx, id, 10)
	if err != nil {
		return AdminUserDetail{}, err
	}
	detail.RecentUsage = usageRows

	ledgerRows, err := r.listRecentLedger(ctx, id, 10)
	if err != nil {
		return AdminUserDetail{}, err
	}
	detail.RecentLedger = ledgerRows

	payments, err := r.listRecentPayments(ctx, id, 10)
	if err != nil {
		return AdminUserDetail{}, err
	}
	detail.RecentPayments = payments

	return detail, nil
}

func (r AdminRepository) UpdateStatus(ctx context.Context, id, status string) (previousStatus string, updated User, err error) {
	if status != StatusActive && status != StatusSuspended {
		return "", User{}, ErrInvalidAdminUserFilter
	}

	err = r.db.QueryRow(ctx, `
		WITH previous AS (
			SELECT status
			FROM users
			WHERE id = $1
			FOR UPDATE
		), updated AS (
			UPDATE users
			SET status = $2
			WHERE id = $1
			RETURNING id::text, email, COALESCE(password_hash, ''), role, status, auth_provider, COALESCE(google_subject, ''), balance_policy, created_at, updated_at
		)
		SELECT previous.status, updated.id, updated.email, updated.password_hash, updated.role, updated.status,
		       updated.auth_provider, updated.google_subject, updated.balance_policy, updated.created_at, updated.updated_at
		FROM previous, updated
	`, id, status).Scan(
		&previousStatus,
		&updated.ID,
		&updated.Email,
		&updated.PasswordHash,
		&updated.Role,
		&updated.Status,
		&updated.AuthProvider,
		&updated.GoogleSubject,
		&updated.BalancePolicy,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", User{}, ErrNotFound
	}
	if err != nil {
		return "", User{}, fmt.Errorf("update user status: %w", err)
	}
	return previousStatus, updated, nil
}

func (r AdminRepository) getLatestAPIKey(ctx context.Context, userID string) (AdminAPIKeySummary, bool, error) {
	var key AdminAPIKeySummary
	err := r.db.QueryRow(ctx, `
		SELECT id::text, key_prefix, status, omniroute_key_id, created_at, last_used_at, revoked_at
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, userID).Scan(
		&key.ID,
		&key.KeyPrefix,
		&key.Status,
		&key.OmniRouteKeyID,
		&key.CreatedAt,
		&key.LastUsedAt,
		&key.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AdminAPIKeySummary{}, false, nil
	}
	if err != nil {
		return AdminAPIKeySummary{}, false, fmt.Errorf("get latest api key summary: %w", err)
	}
	return key, true, nil
}

func (r AdminRepository) listRecentUsage(ctx context.Context, userID string, limit int) ([]AdminUsageSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, api_key_id::text, external_source, external_event_id, omniroute_key_id,
		       model, provider, input_tokens, output_tokens, total_tokens, cost_units, status,
		       occurred_at, created_at
		FROM usage_events
		WHERE user_id = $1
		ORDER BY occurred_at DESC, created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent usage: %w", err)
	}
	defer rows.Close()

	items := []AdminUsageSummary{}
	for rows.Next() {
		var item AdminUsageSummary
		if err := rows.Scan(
			&item.ID,
			&item.APIKeyID,
			&item.ExternalSource,
			&item.ExternalEventID,
			&item.OmniRouteKeyID,
			&item.Model,
			&item.Provider,
			&item.InputTokens,
			&item.OutputTokens,
			&item.TotalTokens,
			&item.CostUnits,
			&item.Status,
			&item.OccurredAt,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recent usage: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r AdminRepository) listRecentLedger(ctx context.Context, userID string, limit int) ([]AdminLedgerSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, type, source, delta_units, balance_after_units, payment_id::text,
		       usage_event_id::text, admin_id::text, idempotency_key, reason, created_at
		FROM credit_ledger_entries
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent ledger: %w", err)
	}
	defer rows.Close()

	items := []AdminLedgerSummary{}
	for rows.Next() {
		var item AdminLedgerSummary
		if err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Source,
			&item.DeltaUnits,
			&item.BalanceAfterUnits,
			&item.PaymentID,
			&item.UsageEventID,
			&item.AdminID,
			&item.IdempotencyKey,
			&item.Reason,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recent ledger: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r AdminRepository) listRecentPayments(ctx context.Context, userID string, limit int) ([]AdminPaymentSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, package_id::text, provider, provider_ref, amount_minor, currency,
		       credit_units, status, admin_id::text, note, created_at, paid_at
		FROM payments
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent payments: %w", err)
	}
	defer rows.Close()

	items := []AdminPaymentSummary{}
	for rows.Next() {
		var item AdminPaymentSummary
		if err := rows.Scan(
			&item.ID,
			&item.PackageID,
			&item.Provider,
			&item.ProviderRef,
			&item.AmountMinor,
			&item.Currency,
			&item.CreditUnits,
			&item.Status,
			&item.AdminID,
			&item.Note,
			&item.CreatedAt,
			&item.PaidAt,
		); err != nil {
			return nil, fmt.Errorf("scan recent payment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func normalizeAdminListFilter(filter AdminListFilter) AdminListFilter {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	filter.Role = strings.ToUpper(strings.TrimSpace(filter.Role))
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}

func validateAdminListFilter(filter AdminListFilter) error {
	if filter.Status != "" && filter.Status != StatusActive && filter.Status != StatusSuspended {
		return ErrInvalidAdminUserFilter
	}
	if filter.Role != "" && filter.Role != RoleUser && filter.Role != RoleAdmin {
		return ErrInvalidAdminUserFilter
	}
	return nil
}

func adminUserWhere(filter AdminListFilter) (string, []any) {
	args := []any{}
	clauses := []string{}
	if filter.Query != "" {
		args = append(args, "%"+NormalizeEmail(filter.Query)+"%")
		clauses = append(clauses, fmt.Sprintf("u.email ILIKE $%d", len(args)))
	}
	if filter.Status != "" {
		args = append(args, filter.Status)
		clauses = append(clauses, fmt.Sprintf("u.status = $%d", len(args)))
	}
	if filter.Role != "" {
		args = append(args, filter.Role)
		clauses = append(clauses, fmt.Sprintf("u.role = $%d", len(args)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

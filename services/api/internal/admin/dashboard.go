package admin

import (
	"context"
	"fmt"
	"time"

	platformdb "github.com/slai/slai/services/api/internal/platform/db"
)

type DashboardRepository struct {
	db platformdb.Executor
}

func NewDashboardRepository(db platformdb.Executor) DashboardRepository {
	return DashboardRepository{db: db}
}

type DashboardResult struct {
	Users           DashboardUsers       `json:"users"`
	Credits         DashboardCredits     `json:"credits"`
	Revenue         DashboardRevenue     `json:"revenue"`
	APIKeys         DashboardAPIKeys     `json:"api_keys"`
	Usage           DashboardUsage       `json:"usage"`
	RecentPayments  []DashboardPayment   `json:"recent_payments"`
	RecentUsage     []DashboardUsageItem `json:"recent_usage"`
	RecentAuditLogs []DashboardAuditLog  `json:"recent_audit_logs"`
	SyncStatus      DashboardSyncStatus  `json:"sync_status"`
}

type DashboardUsers struct {
	Total     int64 `json:"total"`
	Active    int64 `json:"active"`
	Suspended int64 `json:"suspended"`
}

type DashboardCredits struct {
	TotalAvailableUnits int64 `json:"total_available_units"`
	TotalPurchasedUnits int64 `json:"total_purchased_units"`
	TotalUsedUnits      int64 `json:"total_used_units"`
}

type DashboardRevenue struct {
	TotalPaidMinor int64  `json:"total_paid_minor"`
	Currency       string `json:"currency"`
}

type DashboardAPIKeys struct {
	Active    int64 `json:"active"`
	Suspended int64 `json:"suspended"`
	Revoked   int64 `json:"revoked"`
}

type DashboardUsage struct {
	TotalEvents       int64 `json:"total_events"`
	BilledEvents      int64 `json:"billed_events"`
	FailedEvents      int64 `json:"failed_events"`
	IgnoredEvents     int64 `json:"ignored_events"`
	TotalInputTokens  int64 `json:"total_input_tokens"`
	TotalOutputTokens int64 `json:"total_output_tokens"`
	TotalTokens       int64 `json:"total_tokens"`
	TotalCostUnits    int64 `json:"total_cost_units"`
}

type DashboardPayment struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	CreditUnits int64     `json:"credit_units"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type DashboardUsageItem struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	UserEmail   string    `json:"user_email"`
	Model       *string   `json:"model"`
	Provider    *string   `json:"provider"`
	TotalTokens int64     `json:"total_tokens"`
	CostUnits   int64     `json:"cost_units"`
	Status      string    `json:"status"`
	OccurredAt  time.Time `json:"occurred_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type DashboardAuditLog struct {
	ID         string    `json:"id"`
	AdminID    string    `json:"admin_id"`
	AdminEmail string    `json:"admin_email"`
	Action     string    `json:"action"`
	TargetType *string   `json:"target_type"`
	TargetID   *string   `json:"target_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type DashboardSyncStatus struct {
	WorkerEnabled    bool       `json:"worker_enabled"`
	CurrentlyRunning bool       `json:"currently_running"`
	LastSuccessAt    *time.Time `json:"last_success_at"`
	LastError        *string    `json:"last_error"`
}

func (r DashboardRepository) Get(ctx context.Context) (DashboardResult, error) {
	var result DashboardResult
	if err := r.scanUserMetrics(ctx, &result.Users); err != nil {
		return DashboardResult{}, err
	}
	if err := r.scanCreditMetrics(ctx, &result.Credits); err != nil {
		return DashboardResult{}, err
	}
	if err := r.scanRevenueMetrics(ctx, &result.Revenue); err != nil {
		return DashboardResult{}, err
	}
	if err := r.scanAPIKeyMetrics(ctx, &result.APIKeys); err != nil {
		return DashboardResult{}, err
	}
	if err := r.scanUsageMetrics(ctx, &result.Usage); err != nil {
		return DashboardResult{}, err
	}

	payments, err := r.listRecentPayments(ctx, 5)
	if err != nil {
		return DashboardResult{}, err
	}
	result.RecentPayments = payments

	usage, err := r.listRecentUsage(ctx, 5)
	if err != nil {
		return DashboardResult{}, err
	}
	result.RecentUsage = usage

	auditLogs, err := r.listRecentAuditLogs(ctx, 5)
	if err != nil {
		return DashboardResult{}, err
	}
	result.RecentAuditLogs = auditLogs

	return result, nil
}

func (r DashboardRepository) scanUserMetrics(ctx context.Context, dst *DashboardUsers) error {
	err := r.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'ACTIVE'),
		       count(*) FILTER (WHERE status = 'SUSPENDED')
		FROM users
	`).Scan(&dst.Total, &dst.Active, &dst.Suspended)
	if err != nil {
		return fmt.Errorf("scan dashboard user metrics: %w", err)
	}
	return nil
}

func (r DashboardRepository) scanCreditMetrics(ctx context.Context, dst *DashboardCredits) error {
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(available_units), 0),
		       COALESCE(sum(lifetime_purchased_units), 0),
		       COALESCE(sum(lifetime_used_units), 0)
		FROM credit_balances
	`).Scan(&dst.TotalAvailableUnits, &dst.TotalPurchasedUnits, &dst.TotalUsedUnits)
	if err != nil {
		return fmt.Errorf("scan dashboard credit metrics: %w", err)
	}
	return nil
}

func (r DashboardRepository) scanRevenueMetrics(ctx context.Context, dst *DashboardRevenue) error {
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(amount_minor) FILTER (WHERE status = 'paid'), 0),
		       COALESCE(min(currency) FILTER (WHERE status = 'paid'), 'USD')
		FROM payments
	`).Scan(&dst.TotalPaidMinor, &dst.Currency)
	if err != nil {
		return fmt.Errorf("scan dashboard revenue metrics: %w", err)
	}
	return nil
}

func (r DashboardRepository) scanAPIKeyMetrics(ctx context.Context, dst *DashboardAPIKeys) error {
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'ACTIVE'),
		       count(*) FILTER (WHERE status = 'SUSPENDED'),
		       count(*) FILTER (WHERE status = 'REVOKED')
		FROM api_keys
	`).Scan(&dst.Active, &dst.Suspended, &dst.Revoked)
	if err != nil {
		return fmt.Errorf("scan dashboard api key metrics: %w", err)
	}
	return nil
}

func (r DashboardRepository) scanUsageMetrics(ctx context.Context, dst *DashboardUsage) error {
	err := r.db.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE status = 'billed'),
		       count(*) FILTER (WHERE status = 'failed'),
		       count(*) FILTER (WHERE status = 'ignored'),
		       COALESCE(sum(input_tokens), 0),
		       COALESCE(sum(output_tokens), 0),
		       COALESCE(sum(total_tokens), 0),
		       COALESCE(sum(cost_units), 0)
		FROM usage_events
	`).Scan(
		&dst.TotalEvents,
		&dst.BilledEvents,
		&dst.FailedEvents,
		&dst.IgnoredEvents,
		&dst.TotalInputTokens,
		&dst.TotalOutputTokens,
		&dst.TotalTokens,
		&dst.TotalCostUnits,
	)
	if err != nil {
		return fmt.Errorf("scan dashboard usage metrics: %w", err)
	}
	return nil
}

func (r DashboardRepository) listRecentPayments(ctx context.Context, limit int) ([]DashboardPayment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text, p.user_id::text, u.email, p.amount_minor, p.currency,
		       p.credit_units, p.status, p.created_at
		FROM payments p
		JOIN users u ON u.id = p.user_id
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list dashboard recent payments: %w", err)
	}
	defer rows.Close()

	items := []DashboardPayment{}
	for rows.Next() {
		var item DashboardPayment
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.UserEmail,
			&item.AmountMinor,
			&item.Currency,
			&item.CreditUnits,
			&item.Status,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard recent payment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard recent payments: %w", err)
	}
	return items, nil
}

func (r DashboardRepository) listRecentUsage(ctx context.Context, limit int) ([]DashboardUsageItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ue.id::text, ue.user_id::text, u.email, ue.model, ue.provider,
		       ue.total_tokens, ue.cost_units, ue.status, ue.occurred_at, ue.created_at
		FROM usage_events ue
		JOIN users u ON u.id = ue.user_id
		ORDER BY ue.occurred_at DESC, ue.created_at DESC, ue.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list dashboard recent usage: %w", err)
	}
	defer rows.Close()

	items := []DashboardUsageItem{}
	for rows.Next() {
		var item DashboardUsageItem
		if err := rows.Scan(
			&item.ID,
			&item.UserID,
			&item.UserEmail,
			&item.Model,
			&item.Provider,
			&item.TotalTokens,
			&item.CostUnits,
			&item.Status,
			&item.OccurredAt,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard recent usage: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard recent usage: %w", err)
	}
	return items, nil
}

func (r DashboardRepository) listRecentAuditLogs(ctx context.Context, limit int) ([]DashboardAuditLog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT aal.id::text, aal.admin_id::text, u.email, aal.action,
		       aal.target_type, aal.target_id, aal.created_at
		FROM admin_audit_logs aal
		JOIN users u ON u.id = aal.admin_id
		ORDER BY aal.created_at DESC, aal.id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list dashboard recent audit logs: %w", err)
	}
	defer rows.Close()

	items := []DashboardAuditLog{}
	for rows.Next() {
		var item DashboardAuditLog
		if err := rows.Scan(
			&item.ID,
			&item.AdminID,
			&item.AdminEmail,
			&item.Action,
			&item.TargetType,
			&item.TargetID,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard recent audit log: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard recent audit logs: %w", err)
	}
	return items, nil
}

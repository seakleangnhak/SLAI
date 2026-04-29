package users

import "time"

type AdminListFilter struct {
	Query  string
	Status string
	Role   string
	Limit  int
	Offset int
}

type AdminUserListItem struct {
	ID                     string    `json:"id"`
	Email                  string    `json:"email"`
	Role                   string    `json:"role"`
	Status                 string    `json:"status"`
	BalanceUnits           int64     `json:"balance_units"`
	LifetimePurchasedUnits int64     `json:"lifetime_purchased_units"`
	LifetimeUsedUnits      int64     `json:"lifetime_used_units"`
	APIKeyStatus           *string   `json:"api_key_status"`
	APIKeyPrefix           *string   `json:"api_key_prefix"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type AdminUserListResult struct {
	Items  []AdminUserListItem `json:"items"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
	Total  int64               `json:"total"`
}

type AdminBalance struct {
	AvailableUnits         int64     `json:"available_units"`
	LifetimePurchasedUnits int64     `json:"lifetime_purchased_units"`
	LifetimeUsedUnits      int64     `json:"lifetime_used_units"`
	Version                int64     `json:"version"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type AdminAPIKeySummary struct {
	ID             string     `json:"id"`
	KeyPrefix      string     `json:"key_prefix"`
	Status         string     `json:"status"`
	OmniRouteKeyID *string    `json:"omniroute_key_id"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	RevokedAt      *time.Time `json:"revoked_at"`
}

type AdminUsageSummary struct {
	ID              string    `json:"id"`
	APIKeyID        string    `json:"api_key_id"`
	ExternalSource  string    `json:"external_source"`
	ExternalEventID string    `json:"external_event_id"`
	OmniRouteKeyID  *string   `json:"omniroute_key_id"`
	Model           *string   `json:"model"`
	Provider        *string   `json:"provider"`
	InputTokens     int64     `json:"input_tokens"`
	OutputTokens    int64     `json:"output_tokens"`
	TotalTokens     int64     `json:"total_tokens"`
	CostUnits       int64     `json:"cost_units"`
	Status          string    `json:"status"`
	OccurredAt      time.Time `json:"occurred_at"`
	CreatedAt       time.Time `json:"created_at"`
}

type AdminLedgerSummary struct {
	ID                string    `json:"id"`
	Type              string    `json:"type"`
	Source            string    `json:"source"`
	DeltaUnits        int64     `json:"delta_units"`
	BalanceAfterUnits int64     `json:"balance_after_units"`
	PaymentID         *string   `json:"payment_id"`
	UsageEventID      *string   `json:"usage_event_id"`
	AdminID           *string   `json:"admin_id"`
	IdempotencyKey    *string   `json:"idempotency_key"`
	Reason            *string   `json:"reason"`
	CreatedAt         time.Time `json:"created_at"`
}

type AdminPaymentSummary struct {
	ID          string     `json:"id"`
	PackageID   *string    `json:"package_id"`
	Provider    string     `json:"provider"`
	ProviderRef *string    `json:"provider_ref"`
	AmountMinor int64      `json:"amount_minor"`
	Currency    string     `json:"currency"`
	CreditUnits int64      `json:"credit_units"`
	Status      string     `json:"status"`
	AdminID     *string    `json:"admin_id"`
	Note        *string    `json:"note"`
	CreatedAt   time.Time  `json:"created_at"`
	PaidAt      *time.Time `json:"paid_at"`
}

type AdminUserDetail struct {
	ID             string                `json:"id"`
	Email          string                `json:"email"`
	Role           string                `json:"role"`
	Status         string                `json:"status"`
	Balance        AdminBalance          `json:"balance"`
	APIKey         *AdminAPIKeySummary   `json:"api_key"`
	RecentUsage    []AdminUsageSummary   `json:"recent_usage"`
	RecentLedger   []AdminLedgerSummary  `json:"recent_ledger"`
	RecentPayments []AdminPaymentSummary `json:"recent_payments"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

type AdminStatusUpdateInput struct {
	Status string `json:"status"`
}

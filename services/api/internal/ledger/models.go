package ledger

import "time"

const (
	TypePaymentCredit         = "payment_credit"
	TypeUsageDebit            = "usage_debit"
	TypeAdminAdjustmentCredit = "admin_adjustment_credit"
	TypeAdminAdjustmentDebit  = "admin_adjustment_debit"
	TypeRefundDebit           = "refund_debit"
	TypeBonusCredit           = "bonus_credit"

	SourceAdmin  = "admin"
	SourceSystem = "system"
)

type Balance struct {
	UserID                 string    `json:"userId"`
	AvailableUnits         int64     `json:"availableUnits"`
	LifetimePurchasedUnits int64     `json:"lifetimePurchasedUnits"`
	LifetimeUsedUnits      int64     `json:"lifetimeUsedUnits"`
	Version                int64     `json:"version"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

type Entry struct {
	ID                string         `json:"id"`
	UserID            string         `json:"userId"`
	Type              string         `json:"type"`
	Source            string         `json:"source"`
	DeltaUnits        int64          `json:"deltaUnits"`
	BalanceAfterUnits int64          `json:"balanceAfterUnits"`
	PaymentID         *string        `json:"paymentId,omitempty"`
	UsageEventID      *string        `json:"usageEventId,omitempty"`
	AdminID           *string        `json:"adminId,omitempty"`
	IdempotencyKey    *string        `json:"idempotencyKey,omitempty"`
	Reason            *string        `json:"reason,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"createdAt"`
}

type Mutation struct {
	UserID         string
	Type           string
	Source         string
	DeltaUnits     int64
	PaymentID      *string
	UsageEventID   *string
	AdminID        *string
	IdempotencyKey *string
	Reason         *string
	Metadata       map[string]any
}

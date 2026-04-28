package payments

import "time"

const (
	ProviderManual = "manual"
	StatusPaid     = "paid"
)

type Payment struct {
	ID          string     `json:"id"`
	UserID      string     `json:"userId"`
	PackageID   *string    `json:"packageId,omitempty"`
	Provider    string     `json:"provider"`
	ProviderRef *string    `json:"providerRef,omitempty"`
	AmountMinor int64      `json:"amountMinor"`
	Currency    string     `json:"currency"`
	CreditUnits int64      `json:"creditUnits"`
	Status      string     `json:"status"`
	AdminID     *string    `json:"adminId,omitempty"`
	Note        *string    `json:"note,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	PaidAt      *time.Time `json:"paidAt,omitempty"`
}

type ManualTopUpInput struct {
	UserID         string  `json:"userId"`
	PackageID      *string `json:"packageId"`
	AmountMinor    int64   `json:"amountMinor"`
	Currency       string  `json:"currency"`
	CreditUnits    int64   `json:"creditUnits"`
	Note           *string `json:"note"`
	IdempotencyKey *string `json:"idempotencyKey"`
}

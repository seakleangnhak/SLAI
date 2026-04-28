package packages

import "time"

type Package struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      *string   `json:"description,omitempty"`
	CreditUnits      int64     `json:"creditUnits"`
	BonusCreditUnits int64     `json:"bonusCreditUnits"`
	PriceMinor       int64     `json:"priceMinor"`
	Currency         string    `json:"currency"`
	Active           bool      `json:"active"`
	SortOrder        int32     `json:"sortOrder"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type CreatePackageInput struct {
	Name             string  `json:"name"`
	Description      *string `json:"description"`
	CreditUnits      int64   `json:"creditUnits"`
	BonusCreditUnits int64   `json:"bonusCreditUnits"`
	PriceMinor       int64   `json:"priceMinor"`
	Currency         string  `json:"currency"`
	Active           *bool   `json:"active"`
	SortOrder        int32   `json:"sortOrder"`
}

type UpdatePackageInput struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	CreditUnits      *int64  `json:"creditUnits"`
	BonusCreditUnits *int64  `json:"bonusCreditUnits"`
	PriceMinor       *int64  `json:"priceMinor"`
	Currency         *string `json:"currency"`
	Active           *bool   `json:"active"`
	SortOrder        *int32  `json:"sortOrder"`
}

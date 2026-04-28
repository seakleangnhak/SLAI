package usage

import "time"

const (
	StatusPending   = "pending"
	StatusBilled    = "billed"
	StatusDuplicate = "duplicate"
	StatusFailed    = "failed"
	StatusIgnored   = "ignored"

	ExternalSourceMock                  = "mock"
	ExternalSourceOmniRouteCallLogs     = "omniroute_call_logs"
	ExternalSourceOmniRouteUsageHistory = "omniroute_usage_history"
)

type Event struct {
	ID              string         `json:"id"`
	UserID          string         `json:"user_id"`
	APIKeyID        string         `json:"api_key_id"`
	ExternalSource  string         `json:"external_source"`
	ExternalEventID string         `json:"external_event_id"`
	OmniRouteKeyID  *string        `json:"omniroute_key_id"`
	Model           *string        `json:"model"`
	Provider        *string        `json:"provider"`
	InputTokens     int64          `json:"input_tokens"`
	OutputTokens    int64          `json:"output_tokens"`
	TotalTokens     int64          `json:"total_tokens"`
	CostUnits       int64          `json:"cost_units"`
	Status          string         `json:"status"`
	OccurredAt      time.Time      `json:"occurred_at"`
	RawJSON         map[string]any `json:"raw_json,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

type IngestInput struct {
	ExternalSource    string
	ExternalEventID   string
	APIKeyID          *string
	OmniRouteKeyID    *string
	Model             *string
	Provider          *string
	InputTokens       int64
	OutputTokens      int64
	OccurredAt        time.Time
	Raw               map[string]any
	CostUnitsOverride *int64
}

type IngestResult struct {
	Event     *Event `json:"event,omitempty"`
	Status    string `json:"status"`
	Duplicate bool   `json:"duplicate"`
	Ignored   bool   `json:"ignored"`
}

type PricingRule struct {
	ID                   string    `json:"id"`
	Provider             *string   `json:"provider"`
	Model                *string   `json:"model"`
	InputCostUnitsPer1K  int64     `json:"input_cost_units_per_1k"`
	OutputCostUnitsPer1K int64     `json:"output_cost_units_per_1k"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ListFilter struct {
	UserID    *string
	APIKeyID  *string
	Model     *string
	Provider  *string
	Status    *string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Offset    int
}

type SyncResult struct {
	Processed  int `json:"processed"`
	Billed     int `json:"billed"`
	Duplicates int `json:"duplicates"`
	Ignored    int `json:"ignored"`
}

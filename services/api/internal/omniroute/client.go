package omniroute

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/slai/slai/services/api/internal/config"
)

var (
	ErrUnauthorized        = errors.New("omniroute request unauthorized")
	ErrForbidden           = errors.New("omniroute request forbidden")
	ErrNotFound            = errors.New("omniroute resource not found")
	ErrUnsupportedResponse = errors.New("omniroute response is unsupported")
	ErrNotImplemented      = errors.New("omniroute client is a stub; real management API calls are not implemented yet")
)

type Client interface {
	CreateAPIKey(ctx context.Context, name string) (APIKey, error)
	UpdateAPIKey(ctx context.Context, id string, payload UpdateAPIKeyPayload) error
	DeleteAPIKey(ctx context.Context, id string) error
	ListAPIKeys(ctx context.Context) ([]APIKey, error)
	FetchCallLogs(ctx context.Context, since *time.Time, limit int) ([]CallLog, error)
	FetchUsageHistory(ctx context.Context, since *time.Time, limit int) ([]UsageRecord, error)
}

type APIKey struct {
	ID        string
	Name      string
	Prefix    string
	RawKey    string
	MachineID string
	Status    string
	IsActive  *bool
	CreatedAt time.Time
}

type UpdateAPIKeyPayload struct {
	Name               *string
	Status             *string
	IsActive           *bool
	AllowedModels      []string
	AllowedConnections []string
	NoLog              *bool
}

type CallLog struct {
	ExternalID   string
	APIKeyID     string
	Model        string
	Provider     string
	InputTokens  int64
	OutputTokens int64
	OccurredAt   time.Time
	Raw          map[string]any
}

type UsageRecord struct {
	ExternalID string
	APIKeyID   string
	Model      string
	Provider   string
	CostUnits  int64
	OccurredAt time.Time
	Raw        map[string]any
}

type StubClient struct {
	cfg config.OmniRouteConfig
	log *slog.Logger
}

func NewStubClient(cfg config.OmniRouteConfig, logger *slog.Logger) StubClient {
	return StubClient{cfg: cfg, log: logger}
}

func (c StubClient) CreateAPIKey(_ context.Context, name string) (APIKey, error) {
	c.log.Debug("omniroute CreateAPIKey stub called", "name", name, "enabled", c.cfg.Enabled)
	return APIKey{}, ErrNotImplemented
}

func (c StubClient) UpdateAPIKey(_ context.Context, id string, _ UpdateAPIKeyPayload) error {
	c.log.Debug("omniroute UpdateAPIKey stub called", "id", id, "enabled", c.cfg.Enabled)
	return ErrNotImplemented
}

func (c StubClient) DeleteAPIKey(_ context.Context, id string) error {
	c.log.Debug("omniroute DeleteAPIKey stub called", "id", id, "enabled", c.cfg.Enabled)
	return ErrNotImplemented
}

func (c StubClient) ListAPIKeys(_ context.Context) ([]APIKey, error) {
	c.log.Debug("omniroute ListAPIKeys stub called", "enabled", c.cfg.Enabled)
	return nil, ErrNotImplemented
}

func (c StubClient) FetchCallLogs(_ context.Context, since *time.Time, limit int) ([]CallLog, error) {
	c.log.Debug("omniroute FetchCallLogs stub called", "since", since, "limit", limit, "mode", c.cfg.UsageSyncMode)
	return nil, ErrNotImplemented
}

func (c StubClient) FetchUsageHistory(_ context.Context, since *time.Time, limit int) ([]UsageRecord, error) {
	c.log.Debug("omniroute FetchUsageHistory stub called", "since", since, "limit", limit, "mode", c.cfg.UsageSyncMode)
	return nil, ErrNotImplemented
}

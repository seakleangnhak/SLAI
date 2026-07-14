package omniroute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/credits"
)

const (
	apiKeysPath      = "/api/keys"
	callLogsPath     = "/api/usage/call-logs"
	usageHistoryPath = "/api/usage/history"
	maxErrorBody     = 512
)

// HTTPError is returned for non-2xx OmniRoute responses. BodySnippet is sanitized.
type HTTPError struct {
	StatusCode  int
	BodySnippet string
	Err         error
}

func (e HTTPError) Error() string {
	if e.BodySnippet == "" {
		return fmt.Sprintf("omniroute request failed with status %d", e.StatusCode)
	}
	return fmt.Sprintf("omniroute request failed with status %d: %s", e.StatusCode, e.BodySnippet)
}

func (e HTTPError) Unwrap() error {
	return e.Err
}

type HTTPClient struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
	logger     *slog.Logger
}

func NewHTTPClient(cfg config.OmniRouteConfig, logger *slog.Logger) (*HTTPClient, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("OMNIROUTE_BASE_URL is required when OMNIROUTE_ENABLED=true")
	}
	if strings.TrimSpace(cfg.ManagementToken) == "" {
		return nil, errors.New("OMNIROUTE_MANAGEMENT_TOKEN is required when OMNIROUTE_ENABLED=true")
	}

	parsed, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("OMNIROUTE_BASE_URL must be an absolute URL")
	}

	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &HTTPClient{
		baseURL: parsed,
		token:   cfg.ManagementToken,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}, nil
}

func (c *HTTPClient) CreateAPIKey(ctx context.Context, name string) (APIKey, error) {
	var response createAPIKeyResponse
	if err := c.doJSON(ctx, http.MethodPost, apiKeysPath, nil, map[string]any{
		"name":  name,
		"noLog": false,
	}, &response); err != nil {
		return APIKey{}, err
	}

	if response.ID == "" || response.Key == "" {
		return APIKey{}, fmt.Errorf("%w: create api key response missing id or key", ErrUnsupportedResponse)
	}
	createdAt := response.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	return APIKey{
		ID:        response.ID,
		Name:      response.Name,
		RawKey:    response.Key,
		MachineID: response.MachineID,
		CreatedAt: createdAt,
	}, nil
}

func (c *HTTPClient) UpdateAPIKey(ctx context.Context, id string, payload UpdateAPIKeyPayload) error {
	body := map[string]any{}
	if payload.Name != nil {
		body["name"] = *payload.Name
	}
	if payload.IsActive != nil {
		body["isActive"] = *payload.IsActive
	}
	if payload.AllowedModels != nil {
		body["allowedModels"] = payload.AllowedModels
	}
	if payload.AllowedConnections != nil {
		body["allowedConnections"] = payload.AllowedConnections
	}
	if payload.NoLog != nil {
		body["noLog"] = *payload.NoLog
	}
	if payload.Status != nil {
		body["status"] = *payload.Status
	}

	return c.doJSON(ctx, http.MethodPatch, apiKeysPath+"/"+url.PathEscape(id), nil, body, nil)
}

func (c *HTTPClient) DeleteAPIKey(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, apiKeysPath+"/"+url.PathEscape(id), nil, nil, nil)
}

func (c *HTTPClient) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, apiKeysPath, nil, nil, &raw); err != nil {
		return nil, err
	}

	items, err := decodeAPIKeyList(raw)
	if err != nil {
		return nil, err
	}

	keys := make([]APIKey, 0, len(items))
	for _, item := range items {
		createdAt := item.CreatedAt
		keys = append(keys, APIKey{
			ID:        item.ID,
			Name:      item.Name,
			Prefix:    firstNonEmpty(item.Prefix, item.KeyPrefix, prefixFromMaskedKey(item.Key)),
			MachineID: item.MachineID,
			Status:    item.Status,
			IsActive:  item.IsActive,
			CreatedAt: createdAt,
		})
	}
	return keys, nil
}

func (c *HTTPClient) FetchCallLogs(ctx context.Context, since *time.Time, limit int) ([]CallLog, error) {
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}
	// OmniRoute /api/usage/call-logs currently does not support since. SLAI relies on idempotency.
	_ = since

	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, callLogsPath, query, nil, &raw); err != nil {
		return nil, err
	}

	objects, err := decodeObjectArray(raw, "logs", "callLogs")
	if err != nil {
		return nil, err
	}

	logs := make([]CallLog, 0, len(objects))
	for _, object := range objects {
		log, err := mapCallLog(object)
		if err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, nil
}

func (c *HTTPClient) FetchUsageHistory(ctx context.Context, since *time.Time, limit int) ([]UsageRecord, error) {
	query := url.Values{}
	if since != nil {
		query.Set("since", since.UTC().Format(time.RFC3339))
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, usageHistoryPath, query, nil, &raw); err != nil {
		return nil, err
	}

	objects, err := decodeObjectArray(raw, "history", "usage", "records", "items")
	if err != nil {
		return nil, fmt.Errorf("%w: usage history does not expose stable event records; use OMNIROUTE_USAGE_SYNC_MODE=call_logs", ErrUnsupportedResponse)
	}

	records := make([]UsageRecord, 0, len(objects))
	for _, object := range objects {
		record, err := mapUsageRecord(object)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (c *HTTPClient) doJSON(ctx context.Context, method string, path string, query url.Values, body any, dst any) error {
	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(encoded)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.endpoint(path, query), bodyReader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return c.statusError(response)
	}
	if dst == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}

	decoder := json.NewDecoder(response.Body)
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid json response", ErrUnsupportedResponse)
	}
	return nil
}

func (c *HTTPClient) endpoint(path string, query url.Values) string {
	u := *c.baseURL
	basePath := strings.TrimRight(u.Path, "/")
	u.Path = basePath + path
	u.RawQuery = query.Encode()
	return u.String()
}

func (c *HTTPClient) statusError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody+256))
	snippet := sanitizeBodySnippet(string(body), c.token)
	if len(snippet) > maxErrorBody {
		snippet = snippet[:maxErrorBody]
	}

	wrapped := errorForStatus(response.StatusCode)
	return HTTPError{StatusCode: response.StatusCode, BodySnippet: snippet, Err: wrapped}
}

func errorForStatus(status int) error {
	switch status {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusMethodNotAllowed:
		return ErrUnsupportedResponse
	default:
		return fmt.Errorf("omniroute unexpected status %d", status)
	}
}

func sanitizeBodySnippet(body string, token string) string {
	snippet := strings.TrimSpace(body)
	if token != "" {
		snippet = strings.ReplaceAll(snippet, token, "[redacted]")
	}
	snippet = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*bearer\s+)[^\s"']+`).ReplaceAllString(snippet, "${1}[redacted]")
	snippet = regexp.MustCompile(`(?i)("key"\s*:\s*")[^"]+`).ReplaceAllString(snippet, `${1}[redacted]`)
	snippet = regexp.MustCompile(`(?i)("raw[_-]?api[_-]?key"\s*:\s*")[^"]+`).ReplaceAllString(snippet, `${1}[redacted]`)
	return snippet
}

type createAPIKeyResponse struct {
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	ID        string    `json:"id"`
	MachineID string    `json:"machineId"`
	NoLog     bool      `json:"noLog"`
	CreatedAt time.Time `json:"createdAt"`
}

type apiKeyListResponse struct {
	Keys []apiKeyListItem `json:"keys"`
}

type apiKeyListItem struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Prefix    string    `json:"prefix"`
	KeyPrefix string    `json:"keyPrefix"`
	MachineID string    `json:"machineId"`
	Status    string    `json:"status"`
	IsActive  *bool     `json:"isActive"`
	CreatedAt time.Time `json:"createdAt"`
}

func decodeAPIKeyList(raw json.RawMessage) ([]apiKeyListItem, error) {
	var object apiKeyListResponse
	if err := json.Unmarshal(raw, &object); err == nil && object.Keys != nil {
		return object.Keys, nil
	}

	var array []apiKeyListItem
	if err := json.Unmarshal(raw, &array); err == nil {
		return array, nil
	}
	return nil, fmt.Errorf("%w: api key list response must be an array or contain keys", ErrUnsupportedResponse)
}

func decodeObjectArray(raw json.RawMessage, keys ...string) ([]map[string]any, error) {
	var array []map[string]any
	if err := json.Unmarshal(raw, &array); err == nil {
		return array, nil
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%w: expected object or array", ErrUnsupportedResponse)
	}
	for _, key := range keys {
		if value, ok := object[key]; ok {
			if err := json.Unmarshal(value, &array); err != nil {
				return nil, fmt.Errorf("%w: %s is not an array", ErrUnsupportedResponse, key)
			}
			return array, nil
		}
	}
	return nil, fmt.Errorf("%w: response does not contain a supported record array", ErrUnsupportedResponse)
}

func mapCallLog(raw map[string]any) (CallLog, error) {
	externalID := stringField(raw, "id", "requestId", "request_id")
	if externalID == "" {
		return CallLog{}, fmt.Errorf("%w: call log is missing stable id", ErrUnsupportedResponse)
	}

	occurredAt, err := timeField(raw, "timestamp", "createdAt", "occurredAt")
	if err != nil {
		return CallLog{}, err
	}
	costUnits := costUnitsField(raw)

	return CallLog{
		ExternalID:   externalID,
		APIKeyID:     stringField(raw, "apiKeyId", "api_key_id"),
		ComboName:    stringField(raw, "comboName", "combo_name"),
		Model:        stringField(raw, "model", "requestedModel", "requested_model"),
		Provider:     stringField(raw, "provider", "connectionProvider", "connectionName", "connection_id"),
		InputTokens:  int64Field(raw, []string{"inputTokens", "promptTokens", "tokensIn", "tokens_in"}, []string{"tokens", "in"}, []string{"tokens", "input"}),
		OutputTokens: int64Field(raw, []string{"outputTokens", "completionTokens", "tokensOut", "tokens_out"}, []string{"tokens", "out"}, []string{"tokens", "output"}),
		CostUnits:    costUnits,
		OccurredAt:   occurredAt,
		Raw:          raw,
	}, nil
}

func mapUsageRecord(raw map[string]any) (UsageRecord, error) {
	externalID := stringField(raw, "id", "requestId", "request_id", "externalId", "external_event_id")
	apiKeyID := stringField(raw, "apiKeyId", "api_key_id")
	if externalID == "" || apiKeyID == "" {
		return UsageRecord{}, fmt.Errorf("%w: usage history record is missing stable id or apiKeyId", ErrUnsupportedResponse)
	}
	occurredAt, err := timeField(raw, "timestamp", "createdAt", "occurredAt")
	if err != nil {
		return UsageRecord{}, err
	}
	costUnits := int64(0)
	if parsed := costUnitsField(raw); parsed != nil {
		costUnits = *parsed
	}
	return UsageRecord{
		ExternalID: externalID,
		APIKeyID:   apiKeyID,
		ComboName:  stringField(raw, "comboName", "combo_name"),
		Model:      stringField(raw, "model"),
		Provider:   stringField(raw, "provider", "connectionProvider", "connectionName"),
		CostUnits:  costUnits,
		OccurredAt: occurredAt,
		Raw:        raw,
	}, nil
}

func stringField(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case fmt.Stringer:
			return strings.TrimSpace(typed.String())
		default:
			return strings.TrimSpace(fmt.Sprint(typed))
		}
	}
	return ""
}

func int64Field(raw map[string]any, paths ...[]string) int64 {
	for _, path := range paths {
		value, ok := nestedField(raw, path...)
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return int64(typed)
		case float32:
			return int64(typed)
		case int:
			return int64(typed)
		case int64:
			return typed
		case json.Number:
			parsed, _ := typed.Int64()
			return parsed
		case string:
			parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}

func costUnitsField(raw map[string]any) *int64 {
	if value, ok := decimalStringField(raw,
		[]string{"costUnits"},
		[]string{"cost_units"},
		[]string{"creditUnits"},
		[]string{"credit_units"},
		[]string{"cost", "credits"},
	); ok {
		units, err := credits.FromDecimalString(value)
		if err == nil {
			return &units
		}
	}

	if value, ok := decimalStringField(raw,
		[]string{"costUsd"},
		[]string{"cost_usd"},
		[]string{"costUSD"},
		[]string{"usdCost"},
		[]string{"usd_cost"},
		[]string{"cost", "usd"},
		[]string{"cost", "USD"},
		[]string{"billing", "costUsd"},
		[]string{"billing", "cost_usd"},
	); ok {
		units, err := credits.FromDecimalString(value)
		if err == nil {
			return &units
		}
	}

	return nil
}

func decimalStringField(raw map[string]any, paths ...[]string) (string, bool) {
	for _, path := range paths {
		value, ok := nestedField(raw, path...)
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64), true
		case float32:
			return strconv.FormatFloat(float64(typed), 'f', -1, 32), true
		case int:
			return strconv.Itoa(typed), true
		case int64:
			return strconv.FormatInt(typed, 10), true
		case json.Number:
			return typed.String(), true
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed != "" {
				return trimmed, true
			}
		}
	}
	return "", false
}

func nestedField(raw map[string]any, path ...string) (any, bool) {
	if len(path) == 0 {
		return nil, false
	}
	current := any(raw)
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[key]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func timeField(raw map[string]any, keys ...string) (time.Time, error) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			trimmed := strings.TrimSpace(typed)
			if trimmed == "" {
				continue
			}
			parsed, err := time.Parse(time.RFC3339Nano, trimmed)
			if err != nil {
				return time.Time{}, fmt.Errorf("%w: invalid timestamp", ErrUnsupportedResponse)
			}
			return parsed.UTC(), nil
		case float64:
			return unixFlexible(typed), nil
		case int64:
			return unixFlexible(float64(typed)), nil
		case int:
			return unixFlexible(float64(typed)), nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: record is missing timestamp", ErrUnsupportedResponse)
}

func unixFlexible(value float64) time.Time {
	if value > 1_000_000_000_000 {
		return time.UnixMilli(int64(value)).UTC()
	}
	return time.Unix(int64(value), 0).UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func prefixFromMaskedKey(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if idx := strings.IndexAny(trimmed, "*…"); idx > 0 {
		return trimmed[:idx]
	}
	if len(trimmed) > 16 {
		return trimmed[:16]
	}
	return trimmed
}

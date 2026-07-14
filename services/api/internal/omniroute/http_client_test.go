package omniroute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/credits"
)

func TestHTTPClientCreateAPIKeySendsAuthAndMapsResponse(t *testing.T) {
	var authHeader string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != http.MethodPost || r.URL.Path != apiKeysPath {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, w, http.StatusCreated, map[string]any{
			"key":       "omni_raw_abcdefghijklmnopqrstuvwxyz",
			"name":      "SLAI user@example.com",
			"id":        "omni-key-1",
			"machineId": "machine-1",
			"noLog":     false,
			"createdAt": "2026-04-28T10:00:00Z",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	key, err := client.CreateAPIKey(context.Background(), "SLAI user@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if authHeader != "Bearer secret-token" {
		t.Fatalf("Authorization = %q", authHeader)
	}
	if body["name"] != "SLAI user@example.com" || body["noLog"] != false {
		t.Fatalf("body = %#v", body)
	}
	if key.ID != "omni-key-1" || key.Name != "SLAI user@example.com" || key.RawKey != "omni_raw_abcdefghijklmnopqrstuvwxyz" || key.MachineID != "machine-1" {
		t.Fatalf("key = %#v", key)
	}
	if key.CreatedAt.IsZero() {
		t.Fatal("createdAt was not mapped")
	}
}

func TestHTTPClientCreateAPIKeySanitizesSecretsInErrors(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"key":"omni_raw_secret","message":"secret-token"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewHTTPClient(config.OmniRouteConfig{BaseURL: server.URL, ManagementToken: "secret-token", HTTPTimeout: time.Second}, logger)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CreateAPIKey(context.Background(), "Default")
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "omni_raw_secret") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error leaked secret: %v", err)
	}
	if strings.Contains(logBuffer.String(), "omni_raw_secret") || strings.Contains(logBuffer.String(), "secret-token") {
		t.Fatalf("logs leaked secret: %s", logBuffer.String())
	}
}

func TestHTTPClientUpdateAPIKeySendsIsActive(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/api/keys/key-1" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		bodies = append(bodies, body)
		writeJSON(t, w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	inactive := false
	if err := client.UpdateAPIKey(context.Background(), "key-1", UpdateAPIKeyPayload{IsActive: &inactive}); err != nil {
		t.Fatal(err)
	}
	active := true
	if err := client.UpdateAPIKey(context.Background(), "key-1", UpdateAPIKeyPayload{IsActive: &active}); err != nil {
		t.Fatal(err)
	}
	if len(bodies) != 2 || bodies[0]["isActive"] != false || bodies[1]["isActive"] != true {
		t.Fatalf("bodies = %#v", bodies)
	}
}

func TestHTTPClientDeleteAPIKeySendsDELETE(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		path = r.URL.Path
		writeJSON(t, w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	if err := client.DeleteAPIKey(context.Background(), "key-1"); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/api/keys/key-1" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestHTTPClientListAPIKeysSupportsObjectResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"keys": []map[string]any{{
			"id": "key-1", "name": "Default", "key": "sk_live****abcd", "isActive": true, "createdAt": "2026-04-28T10:00:00Z",
		}}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	keys, err := client.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != "key-1" || keys[0].Prefix != "sk_live" || keys[0].IsActive == nil || !*keys[0].IsActive {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestHTTPClientListAPIKeysSupportsRawArrayResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, []map[string]any{{"id": "key-1", "name": "Default", "prefix": "sk_live", "createdAt": "2026-04-28T10:00:00Z"}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	keys, err := client.ListAPIKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].ID != "key-1" || keys[0].Prefix != "sk_live" {
		t.Fatalf("keys = %#v", keys)
	}
}

func TestHTTPClientFetchCallLogsMapsLogs(t *testing.T) {
	var limit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != callLogsPath {
			t.Fatalf("path = %s", r.URL.Path)
		}
		limit = r.URL.Query().Get("limit")
		writeJSON(t, w, http.StatusOK, []map[string]any{{
			"id":        "log-1",
			"apiKeyId":  "omni-key-1",
			"comboName": "fast-combo",
			"model":     "gpt-5.5",
			"provider":  "openai",
			"tokens":    map[string]any{"in": 7240, "out": 357},
			"costUsd":   1.25,
			"timestamp": "2026-04-28T10:00:00Z",
		}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	logs, err := client.FetchCallLogs(context.Background(), nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if limit != "25" {
		t.Fatalf("limit = %q", limit)
	}
	if len(logs) != 1 || logs[0].ExternalID != "log-1" || logs[0].APIKeyID != "omni-key-1" || logs[0].ComboName != "fast-combo" || logs[0].InputTokens != 7240 || logs[0].OutputTokens != 357 {
		t.Fatalf("logs = %#v", logs)
	}
	wantCost, err := credits.FromDecimalString("1.25")
	if err != nil {
		t.Fatal(err)
	}
	if logs[0].CostUnits == nil || *logs[0].CostUnits != wantCost {
		t.Fatalf("cost units = %#v, want %d", logs[0].CostUnits, wantCost)
	}
}

func TestHTTPClientFetchCallLogsMapsSubCentUSDCost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, []map[string]any{{
			"id":        "log-1",
			"apiKeyId":  "omni-key-1",
			"costUsd":   "0.001",
			"timestamp": "2026-04-28T10:00:00Z",
		}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	logs, err := client.FetchCallLogs(context.Background(), nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	wantCost, err := credits.FromDecimalString("0.001")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].CostUnits == nil || *logs[0].CostUnits != wantCost {
		t.Fatalf("logs = %#v, want cost %d", logs, wantCost)
	}
}

func TestHTTPClientFetchCallLogsMapsCreditCostUnits(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, []map[string]any{{
			"id":        "log-1",
			"apiKeyId":  "omni-key-1",
			"costUnits": 7,
			"timestamp": "2026-04-28T10:00:00Z",
		}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	logs, err := client.FetchCallLogs(context.Background(), nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	wantCost, err := credits.FromDecimalString("7")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].CostUnits == nil || *logs[0].CostUnits != wantCost {
		t.Fatalf("logs = %#v, want cost %d", logs, wantCost)
	}
}

func TestHTTPClientFetchCallLogsMissingStableID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, []map[string]any{{"apiKeyId": "key-1", "timestamp": "2026-04-28T10:00:00Z"}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	_, err := client.FetchCallLogs(context.Background(), nil, 10)
	if !errors.Is(err, ErrUnsupportedResponse) {
		t.Fatalf("err = %v, want ErrUnsupportedResponse", err)
	}
}

func TestHTTPClientFetchCallLogsMissingAPIKeyIDIsSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, []map[string]any{{"id": "log-1", "timestamp": "2026-04-28T10:00:00Z"}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	logs, err := client.FetchCallLogs(context.Background(), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].APIKeyID != "" {
		t.Fatalf("logs = %#v", logs)
	}
}

func TestHTTPClientFetchUsageHistoryUnsupportedWhenRequiredFieldsMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"history": []map[string]any{{"timestamp": "2026-04-28T10:00:00Z"}}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	_, err := client.FetchUsageHistory(context.Background(), nil, 10)
	if !errors.Is(err, ErrUnsupportedResponse) {
		t.Fatalf("err = %v, want ErrUnsupportedResponse", err)
	}
}

func TestHTTPClientStatusErrors(t *testing.T) {
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusUnauthorized, ErrUnauthorized},
		{http.StatusForbidden, ErrForbidden},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusMethodNotAllowed, ErrUnsupportedResponse},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, http.StatusText(test.status), test.status)
			}))
			defer server.Close()

			client := newTestClient(t, server.URL, 0)
			_, err := client.ListAPIKeys(context.Background())
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
		})
	}
}

func TestHTTPClientInvalidJSONMapsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{")
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	_, err := client.ListAPIKeys(context.Background())
	if !errors.Is(err, ErrUnsupportedResponse) {
		t.Fatalf("err = %v, want ErrUnsupportedResponse", err)
	}
}

func TestHTTPClientContextCancellationWorks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]any{"keys": []any{}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 0)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.ListAPIKeys(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestHTTPClientTimeoutIsRespected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeJSON(t, w, http.StatusOK, map[string]any{"keys": []any{}})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, 50*time.Millisecond)
	start := time.Now()
	_, err := client.ListAPIKeys(context.Background())
	if err == nil {
		t.Fatal("expected timeout")
	}
	if time.Since(start) > time.Second {
		t.Fatalf("timeout took too long: %s", time.Since(start))
	}
}

func newTestClient(t *testing.T, baseURL string, timeout time.Duration) *HTTPClient {
	t.Helper()
	client, err := NewHTTPClient(config.OmniRouteConfig{
		BaseURL:         baseURL,
		ManagementToken: "secret-token",
		HTTPTimeout:     timeout,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

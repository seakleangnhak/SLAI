package usage_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/apikeys"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/omniroute"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	"github.com/slai/slai/services/api/internal/usage"
	"github.com/slai/slai/services/api/internal/users"
)

var (
	testDB    *pgxpool.Pool
	testDBErr error
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	databaseURL, cleanup, err := startPostgres(ctx)
	if err != nil {
		testDBErr = err
		os.Exit(m.Run())
	}
	defer cleanup()

	pool, err := platformdb.Open(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open test db: %v\n", err)
		os.Exit(1)
	}
	testDB = pool

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "db", "migrations"))
	if err := platformdb.NewMigrator(pool, migrationsDir, slog.New(slog.NewTextHandler(io.Discard, nil))).Up(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func TestCostCalculationExactProviderModelMatch(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	_, err := testDB.Exec(context.Background(), `
		INSERT INTO pricing_rules (provider, model, input_cost_units_per_1k, output_cost_units_per_1k, active)
		VALUES ('openai', 'gpt-5.5', 3, 5, true)
	`)
	if err != nil {
		t.Fatal(err)
	}

	provider := "openai"
	model := "gpt-5.5"
	cost, rule, err := usage.NewPricingService(testDB).CalculateCost(context.Background(), &provider, &model, 1001, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cost != 11 {
		t.Fatalf("cost = %d, want 11", cost)
	}
	if rule.InputCostUnitsPer1K != 3 || rule.OutputCostUnitsPer1K != 5 {
		t.Fatalf("wrong pricing rule selected: %#v", rule)
	}
}

func TestCostCalculationFallsBackToDefault(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	provider := "unknown"
	model := "missing"

	cost, _, err := usage.NewPricingService(testDB).CalculateCost(context.Background(), &provider, &model, 2000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cost != 3 {
		t.Fatalf("cost = %d, want 3", cost)
	}
}

func TestIngestMockUsageDeductsBalanceLedgerAndIsIdempotent(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	key := createLocalAPIKey(t, user.ID)
	creditUser(t, user.ID, 100)
	svc := localUsageService()
	model := "gpt-5.5"
	provider := "openai"

	result, err := svc.IngestMockEvent(context.Background(), usage.IngestInput{
		ExternalEventID: "mock-001",
		APIKeyID:        &key.APIKey.ID,
		Model:           &model,
		Provider:        &provider,
		InputTokens:     7240,
		OutputTokens:    357,
		OccurredAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != usage.StatusBilled || result.Event == nil || result.Event.CostUnits != 9 {
		t.Fatalf("unexpected ingest result: %#v", result)
	}
	assertBalanceAndLifetimeUsed(t, user.ID, 91, 9)
	assertUsageLedger(t, result.Event.ID, -9)

	replay, err := svc.IngestMockEvent(context.Background(), usage.IngestInput{
		ExternalEventID: "mock-001",
		APIKeyID:        &key.APIKey.ID,
		Model:           &model,
		Provider:        &provider,
		InputTokens:     7240,
		OutputTokens:    357,
		OccurredAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Duplicate || replay.Status != usage.StatusDuplicate {
		t.Fatalf("expected duplicate replay, got %#v", replay)
	}
	assertBalanceAndLifetimeUsed(t, user.ID, 91, 9)
}

func TestUnknownAPIKeyIsIgnored(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	creditUser(t, user.ID, 50)
	unknownKeyID := "00000000-0000-0000-0000-000000000001"
	svc := localUsageService()

	result, err := svc.IngestMockEvent(context.Background(), usage.IngestInput{
		ExternalEventID: "unknown-key",
		APIKeyID:        &unknownKeyID,
		InputTokens:     1000,
		OccurredAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Ignored || result.Status != usage.StatusIgnored {
		t.Fatalf("expected ignored result, got %#v", result)
	}
	assertBalanceAndLifetimeUsed(t, user.ID, 50, 0)

	var count int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM usage_events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("usage events count = %d, want 0", count)
	}
}

func TestUsageCanMakeBalanceNegativeAndSuspendsKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	key := createLocalAPIKey(t, user.ID)
	creditUser(t, user.ID, 5)
	svc := localUsageService()

	result, err := svc.IngestMockEvent(context.Background(), usage.IngestInput{
		ExternalEventID: "overrun",
		APIKeyID:        &key.APIKey.ID,
		InputTokens:     5001,
		OccurredAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Event == nil || result.Event.CostUnits != 6 {
		t.Fatalf("unexpected result: %#v", result)
	}
	assertBalanceAndLifetimeUsed(t, user.ID, -1, 6)
	assertKeyStatus(t, key.APIKey.ID, apikeys.StatusSuspended)
}

func TestUsageSuspensionCallsOmniRouteWhenEnabled(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	creditUser(t, user.ID, 1)
	mock := &mockOmniRoute{}
	apiKeys := apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: true}, mock, testLogger())
	created, err := apiKeys.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}
	omniRouteKeyID := "omni_1"
	svc := usage.NewService(testDB, mock, config.OmniRouteConfig{Enabled: true, UsageSyncMode: "call_logs"}, testLogger())

	_, err = svc.IngestEvent(context.Background(), usage.IngestInput{
		ExternalSource:  usage.ExternalSourceOmniRouteCallLogs,
		ExternalEventID: "omni-usage-001",
		OmniRouteKeyID:  &omniRouteKeyID,
		InputTokens:     1001,
		OccurredAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertKeyStatus(t, created.APIKey.ID, apikeys.StatusSuspended)
	if mock.updateCallsValue() != 1 {
		t.Fatalf("omniroute update calls = %d, want 1", mock.updateCallsValue())
	}
}

func TestRealOmniRouteClientBillingFlowWithFakeServer(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	creditUser(t, user.ID, 1)
	fake := newFakeOmniRouteServer(t)
	defer fake.server.Close()

	omniClient, err := omniroute.NewHTTPClient(config.OmniRouteConfig{
		BaseURL:         fake.server.URL,
		ManagementToken: "secret-token",
		HTTPTimeout:     time.Second,
		CallLogLimit:    100,
	}, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	apiKeys := apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: true}, omniClient, testLogger())
	created, err := apiKeys.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if created.RawAPIKey != "omni_raw_abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("raw key = %s", created.RawAPIKey)
	}

	var storedOmniRouteKeyID string
	if err := testDB.QueryRow(context.Background(), `SELECT omniroute_key_id FROM api_keys WHERE id = $1`, created.APIKey.ID).Scan(&storedOmniRouteKeyID); err != nil {
		t.Fatal(err)
	}
	if storedOmniRouteKeyID != "omni-key-1" {
		t.Fatalf("stored omniroute key id = %s", storedOmniRouteKeyID)
	}

	svc := usage.NewService(testDB, omniClient, config.OmniRouteConfig{Enabled: true, UsageSyncMode: "call_logs", CallLogLimit: 100}, testLogger())
	result, err := svc.SyncOmniRoute(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if result.Billed != 1 || result.Fetched != 1 {
		t.Fatalf("sync result = %#v", result)
	}
	assertBalanceAndLifetimeUsed(t, user.ID, -1, 2)
	assertKeyStatus(t, created.APIKey.ID, apikeys.StatusSuspended)
	if fake.patchInactiveCalls != 1 {
		t.Fatalf("patch inactive calls = %d, want 1", fake.patchInactiveCalls)
	}

	replay, err := svc.SyncOmniRoute(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Duplicate != 1 || replay.Billed != 0 {
		t.Fatalf("replay result = %#v", replay)
	}
	assertBalanceAndLifetimeUsed(t, user.ID, -1, 2)
	if fake.patchInactiveCalls != 1 {
		t.Fatalf("duplicate sync patched key again: %d", fake.patchInactiveCalls)
	}
}

func TestPostgresAdvisoryLockPreventsConcurrentAcquisition(t *testing.T) {
	requireDB(t)
	acquired, release, err := platformdb.TryAcquireAdvisoryLock(context.Background(), testDB, "test_usage_sync_multi_replica")
	if err != nil {
		t.Fatal(err)
	}
	if !acquired {
		t.Fatal("first advisory lock acquisition failed")
	}

	secondAcquired, secondRelease, err := platformdb.TryAcquireAdvisoryLock(context.Background(), testDB, "test_usage_sync_multi_replica")
	if err != nil {
		release()
		t.Fatal(err)
	}
	if secondAcquired {
		secondRelease()
		release()
		t.Fatal("second advisory lock acquisition succeeded while first lock was held")
	}

	release()
	thirdAcquired, thirdRelease, err := platformdb.TryAcquireAdvisoryLock(context.Background(), testDB, "test_usage_sync_multi_replica")
	if err != nil {
		t.Fatal(err)
	}
	if !thirdAcquired {
		t.Fatal("advisory lock was not released")
	}
	thirdRelease()
}

func TestSyncStubReturnsNotImplemented(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	svc := usage.NewService(
		testDB,
		omniroute.NewStubClient(config.OmniRouteConfig{UsageSyncMode: "call_logs"}, testLogger()),
		config.OmniRouteConfig{UsageSyncMode: "call_logs"},
		testLogger(),
	)

	_, err := svc.SyncOmniRoute(context.Background(), 100)
	if !errors.Is(err, usage.ErrSyncNotImplemented) {
		t.Fatalf("err = %v, want ErrSyncNotImplemented", err)
	}
}

func localUsageService() usage.Service {
	return usage.NewService(testDB, nil, config.OmniRouteConfig{Enabled: false, UsageSyncMode: "call_logs"}, testLogger())
}

func localAPIKeyService() apikeys.Service {
	return apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: false}, nil, testLogger())
}

func createLocalAPIKey(t *testing.T, userID string) apikeys.CreatedAPIKey {
	t.Helper()
	created, err := localAPIKeyService().CreateAPIKey(context.Background(), userID, "Default")
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func createUser(t *testing.T, email, role string) users.User {
	t.Helper()
	passwordHash, err := auth.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	var created users.User
	err = platformdb.InTx(context.Background(), testDB, func(tx pgx.Tx) error {
		user, err := users.NewRepository(tx).Create(context.Background(), email, passwordHash, role)
		if err != nil {
			return err
		}
		if err := ledger.NewService(tx).EnsureBalance(context.Background(), user.ID); err != nil {
			return err
		}
		created = user
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func creditUser(t *testing.T, userID string, units int64) {
	t.Helper()
	err := platformdb.InTx(context.Background(), testDB, func(tx pgx.Tx) error {
		reason := "test credit"
		_, _, err := ledger.NewService(tx).Mutate(context.Background(), ledger.Mutation{
			UserID:     userID,
			Type:       ledger.TypeAdminAdjustmentCredit,
			Source:     ledger.SourceAdmin,
			DeltaUnits: units,
			AdminID:    &userID,
			Reason:     &reason,
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertBalanceAndLifetimeUsed(t *testing.T, userID string, available int64, lifetimeUsed int64) {
	t.Helper()
	var actualAvailable, actualLifetimeUsed int64
	if err := testDB.QueryRow(context.Background(), `SELECT available_units, lifetime_used_units FROM credit_balances WHERE user_id = $1`, userID).Scan(&actualAvailable, &actualLifetimeUsed); err != nil {
		t.Fatal(err)
	}
	if actualAvailable != available || actualLifetimeUsed != lifetimeUsed {
		t.Fatalf("balance available=%d lifetimeUsed=%d, want available=%d lifetimeUsed=%d", actualAvailable, actualLifetimeUsed, available, lifetimeUsed)
	}
}

func assertUsageLedger(t *testing.T, usageEventID string, delta int64) {
	t.Helper()
	var actualDelta int64
	if err := testDB.QueryRow(context.Background(), `SELECT delta_units FROM credit_ledger_entries WHERE usage_event_id = $1`, usageEventID).Scan(&actualDelta); err != nil {
		t.Fatal(err)
	}
	if actualDelta != delta {
		t.Fatalf("ledger delta = %d, want %d", actualDelta, delta)
	}
}

func assertKeyStatus(t *testing.T, keyID string, status string) {
	t.Helper()
	var actual string
	if err := testDB.QueryRow(context.Background(), `SELECT status FROM api_keys WHERE id = $1`, keyID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != status {
		t.Fatalf("api key status = %s, want %s", actual, status)
	}
}

func truncateTables(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(), `
		TRUNCATE omniroute_sync_state, usage_events, api_keys, admin_audit_logs, credit_ledger_entries, payments, credit_balances, sessions, credit_packages, users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testDB.Exec(context.Background(), `
		DELETE FROM pricing_rules;
		INSERT INTO pricing_rules (provider, model, input_cost_units_per_1k, output_cost_units_per_1k, active)
		VALUES (NULL, NULL, 1, 1, true)
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func requireDB(t *testing.T) {
	t.Helper()
	if testDBErr != nil {
		t.Skipf("PostgreSQL integration test database unavailable: %v", testDBErr)
	}
	if testDB == nil {
		t.Fatal("test database was not initialized")
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type mockOmniRoute struct {
	mu          sync.Mutex
	createCalls int
	updateCalls int
}

func (m *mockOmniRoute) CreateAPIKey(_ context.Context, name string) (omniroute.APIKey, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	return omniroute.APIKey{
		ID:        fmt.Sprintf("omni_%d", m.createCalls),
		Name:      name,
		RawKey:    fmt.Sprintf("omni_raw_%d_abcdefghijklmnopqrstuvwxyz", m.createCalls),
		CreatedAt: time.Now(),
	}, nil
}

func (m *mockOmniRoute) UpdateAPIKey(context.Context, string, omniroute.UpdateAPIKeyPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	return nil
}

func (m *mockOmniRoute) DeleteAPIKey(context.Context, string) error              { return nil }
func (m *mockOmniRoute) ListAPIKeys(context.Context) ([]omniroute.APIKey, error) { return nil, nil }
func (m *mockOmniRoute) FetchCallLogs(context.Context, *time.Time, int) ([]omniroute.CallLog, error) {
	return nil, nil
}
func (m *mockOmniRoute) FetchUsageHistory(context.Context, *time.Time, int) ([]omniroute.UsageRecord, error) {
	return nil, nil
}

func (m *mockOmniRoute) updateCallsValue() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updateCalls
}

type fakeOmniRouteServer struct {
	t                  *testing.T
	server             *httptest.Server
	patchInactiveCalls int
}

func newFakeOmniRouteServer(t *testing.T) *fakeOmniRouteServer {
	t.Helper()
	fake := &fakeOmniRouteServer{t: t}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	return fake
}

func (f *fakeOmniRouteServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer secret-token" {
		f.t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
	}

	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/keys":
		writeTestJSON(f.t, w, http.StatusCreated, map[string]any{
			"key":       "omni_raw_abcdefghijklmnopqrstuvwxyz",
			"name":      "SLAI user@example.com",
			"id":        "omni-key-1",
			"machineId": "machine-1",
		})
	case r.Method == http.MethodGet && r.URL.Path == "/api/usage/call-logs":
		writeTestJSON(f.t, w, http.StatusOK, []map[string]any{{
			"id":        "fake-log-1",
			"apiKeyId":  "omni-key-1",
			"model":     "gpt-5.5",
			"provider":  "openai",
			"tokens":    map[string]any{"in": 1001, "out": 0},
			"timestamp": "2026-04-28T10:00:00Z",
		}})
	case r.Method == http.MethodPatch && r.URL.Path == "/api/keys/omni-key-1":
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Fatal(err)
		}
		if body["isActive"] == false {
			f.patchInactiveCalls++
		}
		writeTestJSON(f.t, w, http.StatusOK, map[string]any{"ok": true})
	default:
		f.t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func startPostgres(ctx context.Context) (string, func(), error) {
	if os.Getenv("SLAI_SKIP_DOCKER_TESTS") == "1" {
		return "", func() {}, fmt.Errorf("SLAI_SKIP_DOCKER_TESTS=1")
	}
	if existing := os.Getenv("SLAI_TEST_DATABASE_URL"); existing != "" {
		return existing, func() {}, nil
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return "", func() {}, err
	}

	name := fmt.Sprintf("slai-usage-test-%d", time.Now().UnixNano())
	password := "slai_test"
	args := []string{
		"run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_DB=slai_test",
		"-e", "POSTGRES_USER=slai_test",
		"-e", "POSTGRES_PASSWORD=" + password,
		"-p", "127.0.0.1::5432",
		"postgres:17-alpine",
	}
	if output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); err != nil {
		return "", func() {}, fmt.Errorf("docker run: %w: %s", err, output)
	}

	cleanup := func() { _ = exec.Command("docker", "rm", "-f", name).Run() }
	portOutput, err := exec.CommandContext(ctx, "docker", "port", name, "5432/tcp").CombinedOutput()
	if err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("docker port: %w: %s", err, portOutput)
	}
	parts := strings.Split(strings.TrimSpace(string(portOutput)), ":")
	port := parts[len(parts)-1]
	databaseURL := fmt.Sprintf("postgres://slai_test:%s@127.0.0.1:%s/slai_test?sslmode=disable", password, port)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			pingErr := pool.Ping(ctx)
			pool.Close()
			if pingErr == nil {
				return databaseURL, cleanup, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}

	cleanup()
	return "", func() {}, fmt.Errorf("postgres test container did not become ready")
}

package httpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	httpserver "github.com/slai/slai/services/api/internal/platform/http"
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

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "..", "..", "db", "migrations"))
	if err := platformdb.NewMigrator(pool, migrationsDir, slog.New(slog.NewTextHandler(io.Discard, nil))).Up(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func TestSignupLoginSessionAuthAndLogout(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)

	signup := client.post(t, "/v1/auth/signup", map[string]any{
		"email":    "Developer@Example.com",
		"password": "correct-password",
	}, nil)
	assertStatus(t, signup, http.StatusCreated)
	if got := stringField(t, signup.JSON, "user.email"); got != "developer@example.com" {
		t.Fatalf("email = %q", got)
	}
	if len(signup.Cookies()) == 0 {
		t.Fatal("expected session cookie on signup")
	}

	me := client.get(t, "/v1/me", signup.Cookies())
	assertStatus(t, me, http.StatusOK)
	if got := stringField(t, me.JSON, "user.role"); got != users.RoleUser {
		t.Fatalf("role = %q", got)
	}

	logout := client.post(t, "/v1/auth/logout", map[string]any{}, signup.Cookies())
	assertStatus(t, logout, http.StatusOK)
	meAfterLogout := client.get(t, "/v1/me", signup.Cookies())
	assertStatus(t, meAfterLogout, http.StatusUnauthorized)

	login := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "developer@example.com",
		"password": "correct-password",
	}, nil)
	assertStatus(t, login, http.StatusOK)

	badLogin := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "developer@example.com",
		"password": "wrong-password",
	}, nil)
	assertStatus(t, badLogin, http.StatusUnauthorized)
}

func TestAdminOnlyAccessAndPackageCreation(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	forbidden := client.get(t, "/v1/admin/packages", userCookies)
	assertStatus(t, forbidden, http.StatusForbidden)

	created := client.post(t, "/v1/admin/packages", map[string]any{
		"name":             "Starter",
		"description":      "Starter credits",
		"creditUnits":      10000,
		"bonusCreditUnits": 500,
		"priceMinor":       1000,
		"currency":         "usd",
		"active":           true,
		"sortOrder":        1,
	}, adminCookies)
	assertStatus(t, created, http.StatusCreated)
	if got := numberField(t, created.JSON, "package.creditUnits"); got != 10000 {
		t.Fatalf("creditUnits = %v", got)
	}

	public := client.get(t, "/v1/packages", nil)
	assertStatus(t, public, http.StatusOK)
	packagesList := public.JSON["packages"].([]any)
	if len(packagesList) != 1 {
		t.Fatalf("public packages len = %d", len(packagesList))
	}
}

func TestAdminUsersListRequiresAdminAndSupportsFilters(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	developer := createUser(t, "developer@example.com", users.RoleUser)
	suspended := createUser(t, "blocked@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, developer.Email)

	if _, err := testDB.Exec(context.Background(), `UPDATE users SET status = $2 WHERE id = $1`, suspended.ID, users.StatusSuspended); err != nil {
		t.Fatal(err)
	}

	createdKey := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, createdKey, http.StatusCreated)

	topup := client.post(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      developer.ID,
		"amountMinor": 1000,
		"currency":    "USD",
		"creditUnits": 5000,
	}, adminCookies)
	assertStatus(t, topup, http.StatusCreated)

	forbidden := client.get(t, "/v1/admin/users", userCookies)
	assertStatus(t, forbidden, http.StatusForbidden)

	list := client.get(t, "/v1/admin/users?q=dev&status=ACTIVE&role=USER&limit=10&offset=0", adminCookies)
	assertStatus(t, list, http.StatusOK)
	if got := numberField(t, list.JSON, "total"); got != 1 {
		t.Fatalf("total = %v", got)
	}
	items := list.JSON["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items len = %d", len(items))
	}
	item := items[0].(map[string]any)
	if got := item["email"]; got != developer.Email {
		t.Fatalf("email = %#v", got)
	}
	if got := item["balance_units"]; got != float64(5000) {
		t.Fatalf("balance_units = %#v", got)
	}
	if got := item["api_key_status"]; got != "ACTIVE" {
		t.Fatalf("api_key_status = %#v", got)
	}
	if _, ok := item["api_key_prefix"].(string); !ok {
		t.Fatalf("api_key_prefix missing: %#v", item)
	}

	suspendedList := client.get(t, "/v1/admin/users?status=SUSPENDED&role=USER", adminCookies)
	assertStatus(t, suspendedList, http.StatusOK)
	if got := numberField(t, suspendedList.JSON, "total"); got != 1 {
		t.Fatalf("suspended total = %v", got)
	}
}

func TestAdminUserDetailDoesNotExposeSecretsAndIncludesRelatedData(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	topup := client.post(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      user.ID,
		"amountMinor": 2500,
		"currency":    "USD",
		"creditUnits": 25000,
	}, adminCookies)
	assertStatus(t, topup, http.StatusCreated)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, created, http.StatusCreated)
	raw := stringField(t, created.JSON, "raw_api_key")
	keyID := stringField(t, created.JSON, "api_key.id")

	ingested := client.post(t, "/v1/internal/usage/mock-event", map[string]any{
		"api_key_id":        keyID,
		"external_event_id": "detail-mock-001",
		"model":             "gpt-5.5",
		"provider":          "openai",
		"input_tokens":      1001,
		"output_tokens":     1,
		"occurred_at":       "2026-04-28T10:00:00Z",
	}, adminCookies)
	assertStatus(t, ingested, http.StatusCreated)

	detail := client.get(t, "/v1/admin/users/"+user.ID, adminCookies)
	assertStatus(t, detail, http.StatusOK)
	for _, forbidden := range []string{"password_hash", "token_hash", "key_hash", "raw_api_key", raw, "API_KEY_PEPPER", "OMNIROUTE_MANAGEMENT_TOKEN"} {
		if strings.Contains(detail.Body, forbidden) {
			t.Fatalf("admin user detail leaked %q: %s", forbidden, detail.Body)
		}
	}
	if got := stringField(t, detail.JSON, "email"); got != user.Email {
		t.Fatalf("email = %q", got)
	}
	if got := numberField(t, detail.JSON, "balance.lifetime_purchased_units"); got != 25000 {
		t.Fatalf("lifetime_purchased_units = %v", got)
	}
	if got := stringField(t, detail.JSON, "api_key.status"); got != "ACTIVE" {
		t.Fatalf("api key status = %q", got)
	}
	if len(detail.JSON["recent_usage"].([]any)) != 1 {
		t.Fatalf("recent_usage = %#v", detail.JSON["recent_usage"])
	}
	if len(detail.JSON["recent_payments"].([]any)) != 1 {
		t.Fatalf("recent_payments = %#v", detail.JSON["recent_payments"])
	}
	if len(detail.JSON["recent_ledger"].([]any)) < 2 {
		t.Fatalf("recent_ledger = %#v", detail.JSON["recent_ledger"])
	}
}

func TestAdminUserStatusSuspendAuditsAndSuspendsAPIKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, created, http.StatusCreated)

	updated := client.patch(t, "/v1/admin/users/"+user.ID+"/status", map[string]any{"status": "SUSPENDED"}, adminCookies)
	assertStatus(t, updated, http.StatusOK)
	if got := stringField(t, updated.JSON, "status"); got != users.StatusSuspended {
		t.Fatalf("status = %q", got)
	}
	if got := stringField(t, updated.JSON, "api_key.status"); got != "SUSPENDED" {
		t.Fatalf("api key status = %q", got)
	}

	var auditCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_logs WHERE admin_id = $1 AND action = 'user_status_updated'`, adminUser.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d", auditCount)
	}

	var keyStatus string
	if err := testDB.QueryRow(context.Background(), `SELECT status FROM api_keys WHERE user_id = $1`, user.ID).Scan(&keyStatus); err != nil {
		t.Fatal(err)
	}
	if keyStatus != "SUSPENDED" {
		t.Fatalf("db api key status = %q", keyStatus)
	}
}

func TestAdminUserStatusActivateDoesNotResumeAPIKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, created, http.StatusCreated)

	suspended := client.patch(t, "/v1/admin/users/"+user.ID+"/status", map[string]any{"status": "SUSPENDED"}, adminCookies)
	assertStatus(t, suspended, http.StatusOK)

	activated := client.patch(t, "/v1/admin/users/"+user.ID+"/status", map[string]any{"status": "ACTIVE"}, adminCookies)
	assertStatus(t, activated, http.StatusOK)
	if got := stringField(t, activated.JSON, "status"); got != users.StatusActive {
		t.Fatalf("status = %q", got)
	}
	if got := stringField(t, activated.JSON, "api_key.status"); got != "SUSPENDED" {
		t.Fatalf("api key should not auto-resume, got %q", got)
	}

	var auditCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_logs WHERE admin_id = $1 AND action = 'user_status_updated'`, adminUser.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 2 {
		t.Fatalf("audit count = %d", auditCount)
	}
}

func TestManualTopUpLedgerMutationAndBalanceUpdate(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)

	topup := client.postWithHeaders(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      user.ID,
		"amountMinor": 2500,
		"currency":    "USD",
		"creditUnits": 25000,
		"note":        "wire received",
	}, adminCookies, map[string]string{"Idempotency-Key": "topup-1"})
	assertStatus(t, topup, http.StatusCreated)
	if got := numberField(t, topup.JSON, "balance.availableUnits"); got != 25000 {
		t.Fatalf("balance.availableUnits = %v", got)
	}
	if got := stringField(t, topup.JSON, "ledger.type"); got != ledger.TypePaymentCredit {
		t.Fatalf("ledger.type = %q", got)
	}

	replay := client.postWithHeaders(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      user.ID,
		"amountMinor": 2500,
		"currency":    "USD",
		"creditUnits": 25000,
	}, adminCookies, map[string]string{"Idempotency-Key": "topup-1"})
	assertStatus(t, replay, http.StatusCreated)
	if got := numberField(t, replay.JSON, "balance.availableUnits"); got != 25000 {
		t.Fatalf("idempotent replay balance.availableUnits = %v", got)
	}

	var ledgerCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM credit_ledger_entries WHERE user_id = $1`, user.ID).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 1 {
		t.Fatalf("ledger count = %d", ledgerCount)
	}

	var paymentCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM payments WHERE user_id = $1`, user.ID).Scan(&paymentCount); err != nil {
		t.Fatal(err)
	}
	if paymentCount != 1 {
		t.Fatalf("payment count = %d", paymentCount)
	}

	var auditCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_logs WHERE admin_id = $1`, adminUser.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d", auditCount)
	}
}

func TestAdjustmentRequiresReasonAndWritesLedgerBalanceAudit(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)

	missingReason := client.post(t, "/v1/admin/ledger/adjustments", map[string]any{
		"userId":     user.ID,
		"deltaUnits": 1000,
	}, adminCookies)
	assertStatus(t, missingReason, http.StatusBadRequest)

	adjustment := client.post(t, "/v1/admin/ledger/adjustments", map[string]any{
		"userId":     user.ID,
		"deltaUnits": 1000,
		"reason":     "customer goodwill credit",
	}, adminCookies)
	assertStatus(t, adjustment, http.StatusCreated)
	if got := stringField(t, adjustment.JSON, "ledger.type"); got != ledger.TypeAdminAdjustmentCredit {
		t.Fatalf("ledger.type = %q", got)
	}
	if got := numberField(t, adjustment.JSON, "balance.availableUnits"); got != 1000 {
		t.Fatalf("balance.availableUnits = %v", got)
	}

	ledgerResponse := client.get(t, "/v1/ledger", loginCookies(t, client, user.Email))
	assertStatus(t, ledgerResponse, http.StatusOK)
	entries := ledgerResponse.JSON["ledger"].([]any)
	if len(entries) != 1 {
		t.Fatalf("ledger entries len = %d", len(entries))
	}

	var auditCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_logs WHERE admin_id = $1`, adminUser.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("audit count = %d", auditCount)
	}
}

func TestPreventBalanceMutationOutsideLedgerService(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)

	_, err := testDB.Exec(context.Background(), `UPDATE credit_balances SET available_units = 999 WHERE user_id = $1`, user.ID)
	if err == nil {
		t.Fatal("expected direct balance update to fail")
	}
	if !strings.Contains(err.Error(), "credit_balances may only be mutated through the ledger service") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertBalance(t, user.ID, 0)
}

func TestLedgerServiceMutation(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)

	err := platformdb.InTx(context.Background(), testDB, func(tx pgx.Tx) error {
		reason := "test credit"
		_, balance, err := ledger.NewService(tx).Mutate(context.Background(), ledger.Mutation{
			UserID:     user.ID,
			Type:       ledger.TypeAdminAdjustmentCredit,
			Source:     ledger.SourceAdmin,
			DeltaUnits: 500,
			AdminID:    &user.ID,
			Reason:     &reason,
		})
		if err != nil {
			return err
		}
		if balance.AvailableUnits != 500 {
			return fmt.Errorf("balance = %d", balance.AvailableUnits)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	assertBalance(t, user.ID, 500)
}

func TestAPIKeyGETDoesNotReturnRawKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	cookies := loginCookies(t, client, user.Email)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, cookies)
	assertStatus(t, created, http.StatusCreated)
	raw := stringField(t, created.JSON, "raw_api_key")
	if raw == "" {
		t.Fatal("create did not return raw api key")
	}

	get := client.get(t, "/v1/api-key", cookies)
	assertStatus(t, get, http.StatusOK)
	if strings.Contains(get.Body, raw) || strings.Contains(get.Body, "raw_api_key") || strings.Contains(get.Body, "key_hash") {
		t.Fatalf("GET leaked raw key or hash: %s", get.Body)
	}
}

func TestMockUsageEndpointAndUsageListDoNotExposeRawKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, created, http.StatusCreated)
	raw := stringField(t, created.JSON, "raw_api_key")
	keyID := stringField(t, created.JSON, "api_key.id")

	ingested := client.post(t, "/v1/internal/usage/mock-event", map[string]any{
		"api_key_id":        keyID,
		"external_event_id": "mock-http-001",
		"model":             "gpt-5.5",
		"provider":          "openai",
		"input_tokens":      1001,
		"output_tokens":     1,
		"occurred_at":       "2026-04-28T10:00:00Z",
	}, adminCookies)
	assertStatus(t, ingested, http.StatusCreated)
	if got := stringField(t, ingested.JSON, "status"); got != "billed" {
		t.Fatalf("ingest status = %q", got)
	}

	usageList := client.get(t, "/v1/usage", userCookies)
	assertStatus(t, usageList, http.StatusOK)
	if strings.Contains(usageList.Body, raw) || strings.Contains(usageList.Body, "raw_api_key") || strings.Contains(usageList.Body, "key_hash") {
		t.Fatalf("usage list leaked raw key or hash: %s", usageList.Body)
	}

	adminList := client.get(t, "/v1/admin/usage?user_id="+user.ID, adminCookies)
	assertStatus(t, adminList, http.StatusOK)
	if strings.Contains(adminList.Body, raw) || strings.Contains(adminList.Body, "raw_api_key") || strings.Contains(adminList.Body, "key_hash") {
		t.Fatalf("admin usage list leaked raw key or hash: %s", adminList.Body)
	}
}

func TestAdminUsageSyncStatusAndManualSyncUpdatesStatus(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	forbidden := client.get(t, "/v1/admin/usage/sync-status", userCookies)
	assertStatus(t, forbidden, http.StatusForbidden)

	before := client.get(t, "/v1/admin/usage/sync-status", adminCookies)
	assertStatus(t, before, http.StatusOK)
	if got := boolField(t, before.JSON, "sync_status.worker_enabled"); got {
		t.Fatal("worker should be disabled in tests")
	}

	response := client.post(t, "/v1/admin/usage/sync", map[string]any{}, adminCookies)
	assertStatus(t, response, http.StatusNotImplemented)

	after := client.get(t, "/v1/admin/usage/sync-status", adminCookies)
	assertStatus(t, after, http.StatusOK)
	status := after.JSON["sync_status"].(map[string]any)
	if status["currently_running"] != false {
		t.Fatalf("currently_running = %#v", status["currently_running"])
	}
	if status["last_started_at"] == nil || status["last_finished_at"] == nil {
		t.Fatalf("manual sync did not update timestamps: %#v", status)
	}
	if status["last_error"] == nil {
		t.Fatalf("manual sync did not record last_error: %#v", status)
	}
	if status["last_result"] == nil {
		t.Fatalf("manual sync did not record last_result: %#v", status)
	}
}

func TestAdminUsageSyncStubReturns501(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	adminCookies := loginCookies(t, client, adminUser.Email)

	response := client.post(t, "/v1/admin/usage/sync", map[string]any{}, adminCookies)
	assertStatus(t, response, http.StatusNotImplemented)
}

type testClient struct {
	server *httptest.Server
	client *http.Client
}

type testResponse struct {
	StatusCode int
	Header     http.Header
	Body       string
	JSON       map[string]any
}

func newTestClient(t *testing.T) testClient {
	t.Helper()
	server := httpserver.NewServer(httpserver.ServerConfig{
		Addr:             ":0",
		ReadinessTimeout: time.Second,
		SessionSecret:    "test-secret",
		CookieSecure:     false,
		SessionTTL:       time.Hour,
		APIKeyPepper:     "test-api-key-pepper",
		APIKeyPrefix:     "sk_slai",
	}, testDB, slog.New(slog.NewTextHandler(io.Discard, nil)))

	return testClient{
		server: httptest.NewServer(server),
		client: &http.Client{},
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

func (c testClient) post(t *testing.T, path string, payload any, cookies []*http.Cookie) testResponse {
	t.Helper()
	return c.postWithHeaders(t, path, payload, cookies, nil)
}

func (c testClient) postWithHeaders(t *testing.T, path string, payload any, cookies []*http.Cookie, headers map[string]string) testResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, c.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return c.do(t, req)
}

func (c testClient) patch(t *testing.T, path string, payload any, cookies []*http.Cookie) testResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return c.do(t, req)
}

func (c testClient) get(t *testing.T, path string, cookies []*http.Cookie) testResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return c.do(t, req)
}

func (c testClient) do(t *testing.T, req *http.Request) testResponse {
	t.Helper()
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	result := testResponse{StatusCode: resp.StatusCode, Header: resp.Header, Body: string(body)}
	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") && len(body) > 0 {
		if err := json.Unmarshal(body, &result.JSON); err != nil {
			t.Fatalf("decode json %s: %v", body, err)
		}
	}
	return result
}

func (r testResponse) Cookies() []*http.Cookie {
	return (&http.Response{Header: r.Header}).Cookies()
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

func loginCookies(t *testing.T, client testClient, email string) []*http.Cookie {
	t.Helper()
	resp := client.post(t, "/v1/auth/login", map[string]any{"email": email, "password": "correct-password"}, nil)
	assertStatus(t, resp, http.StatusOK)
	return resp.Cookies()
}

func truncateTables(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(), `
		TRUNCATE admin_audit_logs, credit_ledger_entries, payments, credit_balances, sessions, credit_packages, users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func assertBalance(t *testing.T, userID string, expected int64) {
	t.Helper()
	var actual int64
	if err := testDB.QueryRow(context.Background(), `SELECT available_units FROM credit_balances WHERE user_id = $1`, userID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("balance = %d, want %d", actual, expected)
	}
}

func assertStatus(t *testing.T, response testResponse, expected int) {
	t.Helper()
	if response.StatusCode != expected {
		t.Fatalf("status = %d, want %d body=%s", response.StatusCode, expected, response.Body)
	}
}

func stringField(t *testing.T, value map[string]any, path string) string {
	t.Helper()
	current := any(value)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object at %s", path, part)
		}
		current = object[part]
	}
	result, ok := current.(string)
	if !ok {
		t.Fatalf("%s is not a string: %#v", path, current)
	}
	return result
}

func boolField(t *testing.T, value map[string]any, path string) bool {
	t.Helper()
	current := any(value)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object at %s", path, part)
		}
		current = object[part]
	}
	result, ok := current.(bool)
	if !ok {
		t.Fatalf("%s is not a bool: %#v", path, current)
	}
	return result
}

func numberField(t *testing.T, value map[string]any, path string) float64 {
	t.Helper()
	current := any(value)
	for _, part := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is not an object at %s", path, part)
		}
		current = object[part]
	}
	result, ok := current.(float64)
	if !ok {
		t.Fatalf("%s is not a number: %#v", path, current)
	}
	return result
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

	name := fmt.Sprintf("slai-test-%d", time.Now().UnixNano())
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

	cleanup := func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	}

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

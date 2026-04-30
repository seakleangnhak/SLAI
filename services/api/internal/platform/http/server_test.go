package httpserver_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	httpserver "github.com/slai/slai/services/api/internal/platform/http"
	"github.com/slai/slai/services/api/internal/slaipayment"
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

func TestAdminDashboardRequiresAdminAndReturnsMetrics(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	forbidden := client.get(t, "/v1/admin/dashboard", userCookies)
	assertStatus(t, forbidden, http.StatusForbidden)

	topup := client.post(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      user.ID,
		"amountMinor": 2500,
		"currency":    "USD",
		"creditUnits": 2500000000,
	}, adminCookies)
	assertStatus(t, topup, http.StatusCreated)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, created, http.StatusCreated)
	raw := stringField(t, created.JSON, "raw_api_key")
	keyID := stringField(t, created.JSON, "api_key.id")

	ingested := client.post(t, "/v1/internal/usage/mock-event", map[string]any{
		"api_key_id":        keyID,
		"external_event_id": "dashboard-mock-001",
		"model":             "gpt-5.5",
		"provider":          "openai",
		"input_tokens":      1001,
		"output_tokens":     1,
		"occurred_at":       "2026-04-28T10:00:00Z",
	}, adminCookies)
	assertStatus(t, ingested, http.StatusCreated)

	dashboard := client.get(t, "/v1/admin/dashboard", adminCookies)
	assertStatus(t, dashboard, http.StatusOK)

	for _, group := range []string{"users", "credits", "revenue", "api_keys", "usage", "sync_status"} {
		if _, ok := dashboard.JSON[group].(map[string]any); !ok {
			t.Fatalf("dashboard missing group %s: %#v", group, dashboard.JSON[group])
		}
	}
	if got := numberField(t, dashboard.JSON, "users.total"); got != 2 {
		t.Fatalf("users.total = %v", got)
	}
	if got := numberField(t, dashboard.JSON, "credits.total_purchased_units"); got != 2500000000 {
		t.Fatalf("credits.total_purchased_units = %v", got)
	}
	if got := numberField(t, dashboard.JSON, "revenue.total_paid_minor"); got != 2500 {
		t.Fatalf("revenue.total_paid_minor = %v", got)
	}
	if got := stringField(t, dashboard.JSON, "revenue.currency"); got != "USD" {
		t.Fatalf("revenue.currency = %q", got)
	}
	if got := numberField(t, dashboard.JSON, "api_keys.active"); got != 1 {
		t.Fatalf("api_keys.active = %v", got)
	}
	if got := numberField(t, dashboard.JSON, "usage.total_events"); got != 1 {
		t.Fatalf("usage.total_events = %v", got)
	}
	if got := numberField(t, dashboard.JSON, "usage.billed_events"); got != 1 {
		t.Fatalf("usage.billed_events = %v", got)
	}

	recentPayments := dashboard.JSON["recent_payments"].([]any)
	if len(recentPayments) != 1 {
		t.Fatalf("recent_payments len = %d", len(recentPayments))
	}
	if got := recentPayments[0].(map[string]any)["user_email"]; got != user.Email {
		t.Fatalf("recent payment user_email = %#v", got)
	}
	recentUsage := dashboard.JSON["recent_usage"].([]any)
	if len(recentUsage) != 1 {
		t.Fatalf("recent_usage len = %d", len(recentUsage))
	}
	if got := recentUsage[0].(map[string]any)["user_email"]; got != user.Email {
		t.Fatalf("recent usage user_email = %#v", got)
	}
	recentAudit := dashboard.JSON["recent_audit_logs"].([]any)
	if len(recentAudit) == 0 {
		t.Fatal("expected recent audit logs")
	}
	if got := recentAudit[0].(map[string]any)["admin_email"]; got != adminUser.Email {
		t.Fatalf("recent audit admin_email = %#v", got)
	}

	for _, forbidden := range []string{"password_hash", "token_hash", "key_hash", "raw_api_key", raw, "API_KEY_PEPPER", "OMNIROUTE_MANAGEMENT_TOKEN"} {
		if strings.Contains(dashboard.Body, forbidden) {
			t.Fatalf("dashboard leaked %q: %s", forbidden, dashboard.Body)
		}
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
		"creditUnits": 2500000000,
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
	if got := numberField(t, detail.JSON, "balance.lifetime_purchased_units"); got != 2500000000 {
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

func TestAdminAuditLogsListRequiresAdminSupportsFiltersAndSanitizesMetadata(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	insertAudit := func(action, targetType, targetID string, metadata map[string]any, createdAt time.Time) {
		t.Helper()
		metadataBytes, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		_, err = testDB.Exec(context.Background(), `
			INSERT INTO admin_audit_logs (admin_id, action, target_type, target_id, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		`, adminUser.ID, action, targetType, targetID, string(metadataBytes), createdAt)
		if err != nil {
			t.Fatal(err)
		}
	}

	baseTime := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	insertAudit("package_created", "credit_package", "pkg-1", map[string]any{
		"name":         "Starter",
		"passwordHash": "dont-leak-password",
		"nested":       map[string]any{"token": "dont-leak-token", "safe": "safe-value"},
		"raw_api_key":  "dont-leak-raw-key",
		"apiKeyPepper": "dont-leak-pepper",
	}, baseTime)
	insertAudit("manual_topup_created", "payment", "pay-1", map[string]any{
		"userId":      user.ID,
		"creditUnits": 5000,
	}, baseTime.Add(time.Minute))
	insertAudit("api_key_suspended", "api_key", "key-1", map[string]any{
		"userId":                   user.ID,
		"omnirouteManagementToken": "dont-leak-management-token",
	}, baseTime.Add(2*time.Minute))

	forbidden := client.get(t, "/v1/admin/audit-logs", userCookies)
	assertStatus(t, forbidden, http.StatusForbidden)

	list := client.get(t, "/v1/admin/audit-logs?limit=100&offset=0", adminCookies)
	assertStatus(t, list, http.StatusOK)
	if got := numberField(t, list.JSON, "total"); got != 3 {
		t.Fatalf("total = %v", got)
	}
	items := list.JSON["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items len = %d", len(items))
	}
	first := items[0].(map[string]any)
	if got := first["admin_email"]; got != adminUser.Email {
		t.Fatalf("admin_email = %#v", got)
	}

	for _, forbiddenValue := range []string{
		"dont-leak-password",
		"dont-leak-token",
		"dont-leak-raw-key",
		"dont-leak-pepper",
		"dont-leak-management-token",
		"password_hash",
		"token_hash",
		"key_hash",
		"raw_api_key\":\"dont-leak-raw-key",
	} {
		if strings.Contains(list.Body, forbiddenValue) {
			t.Fatalf("audit log list leaked %q: %s", forbiddenValue, list.Body)
		}
	}

	actionFiltered := client.get(t, "/v1/admin/audit-logs?action=manual_topup_created", adminCookies)
	assertStatus(t, actionFiltered, http.StatusOK)
	if got := numberField(t, actionFiltered.JSON, "total"); got != 1 {
		t.Fatalf("action filter total = %v", got)
	}
	if got := stringField(t, actionFiltered.JSON["items"].([]any)[0].(map[string]any), "action"); got != "manual_topup_created" {
		t.Fatalf("action = %q", got)
	}

	targetTypeFiltered := client.get(t, "/v1/admin/audit-logs?target_type=payment", adminCookies)
	assertStatus(t, targetTypeFiltered, http.StatusOK)
	if got := numberField(t, targetTypeFiltered.JSON, "total"); got != 1 {
		t.Fatalf("target_type filter total = %v", got)
	}

	targetIDFiltered := client.get(t, "/v1/admin/audit-logs?target_id=key-1", adminCookies)
	assertStatus(t, targetIDFiltered, http.StatusOK)
	if got := numberField(t, targetIDFiltered.JSON, "total"); got != 1 {
		t.Fatalf("target_id filter total = %v", got)
	}

	page := client.get(t, "/v1/admin/audit-logs?limit=1&offset=1", adminCookies)
	assertStatus(t, page, http.StatusOK)
	if got := numberField(t, page.JSON, "total"); got != 3 {
		t.Fatalf("pagination total = %v", got)
	}
	if items := page.JSON["items"].([]any); len(items) != 1 {
		t.Fatalf("pagination items len = %d", len(items))
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

func TestBakongKHQRManualPaymentFlow(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "bakong-user@example.com", users.RoleUser)
	otherUser := createUser(t, "other-bakong-user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)
	otherCookies := loginCookies(t, client, otherUser.Email)

	pkgResp := client.post(t, "/v1/admin/packages", map[string]any{
		"name":             "Starter",
		"description":      "Starter package",
		"creditUnits":      1000000,
		"bonusCreditUnits": 250000,
		"priceMinor":       100,
		"currency":         "USD",
		"active":           true,
	}, adminCookies)
	assertStatus(t, pkgResp, http.StatusCreated)
	packageID := stringField(t, pkgResp.JSON, "package.id")

	disabledCheckout := client.post(t, "/v1/checkout/package/"+packageID, map[string]any{}, userCookies)
	assertStatus(t, disabledCheckout, http.StatusBadRequest)

	forbiddenSettings := client.patch(t, "/v1/admin/payment-settings/bakong-khqr", map[string]any{
		"enabled":      true,
		"display_name": "Bakong KHQR",
		"account_name": "SLAI Co",
		"account_id":   "012345678",
	}, userCookies)
	assertStatus(t, forbiddenSettings, http.StatusForbidden)

	invalidSettings := client.patch(t, "/v1/admin/payment-settings/bakong-khqr", map[string]any{
		"enabled":      true,
		"display_name": "Bakong KHQR",
	}, adminCookies)
	assertStatus(t, invalidSettings, http.StatusBadRequest)

	settings := client.patch(t, "/v1/admin/payment-settings/bakong-khqr", map[string]any{
		"enabled":      true,
		"display_name": "Bakong KHQR",
		"account_name": "SLAI Co",
		"account_id":   "012345678",
		"instructions": "Scan and upload proof.",
	}, adminCookies)
	assertStatus(t, settings, http.StatusOK)

	invalidQR := client.postMultipart(t, "/v1/admin/payment-settings/bakong-khqr/khqr-image", map[string]string{}, "qr.txt", "text/plain", []byte("not an image"), adminCookies)
	assertStatus(t, invalidQR, http.StatusBadRequest)

	qr := client.postMultipart(t, "/v1/admin/payment-settings/bakong-khqr/khqr-image", map[string]string{}, "qr.png", "image/png", tinyPNG(), adminCookies)
	assertStatus(t, qr, http.StatusOK)
	if got := stringField(t, qr.JSON, "settings.khqr_image_url"); got != "/v1/payment-settings/bakong-khqr/khqr-image" {
		t.Fatalf("khqr_image_url = %q", got)
	}
	if strings.Contains(qr.Body, "khqr_image_path") || strings.Contains(qr.Body, "file_path") {
		t.Fatalf("settings response leaked storage path: %s", qr.Body)
	}

	checkout := client.post(t, "/v1/checkout/package/"+packageID, map[string]any{}, userCookies)
	assertStatus(t, checkout, http.StatusCreated)
	paymentID := stringField(t, checkout.JSON, "payment.id")
	if got := stringField(t, checkout.JSON, "payment.status"); got != "pending_proof" {
		t.Fatalf("checkout status = %q", got)
	}

	otherProof := client.postMultipart(t, "/v1/payments/"+paymentID+"/proof", map[string]string{}, "proof.png", "image/png", tinyPNG(), otherCookies)
	assertStatus(t, otherProof, http.StatusForbidden)

	proof := client.postMultipart(t, "/v1/payments/"+paymentID+"/proof", map[string]string{
		"transaction_ref": "user typed ref - not trusted",
		"note":            "paid from mobile app",
	}, "proof.png", "image/png", tinyPNG(), userCookies)
	assertStatus(t, proof, http.StatusOK)
	if got := stringField(t, proof.JSON, "payment.status"); got != "pending_review" {
		t.Fatalf("proof status = %q", got)
	}

	userProof := client.get(t, "/v1/payments/"+paymentID+"/proof", userCookies)
	assertStatus(t, userProof, http.StatusOK)
	if !strings.Contains(userProof.Header.Get("Content-Type"), "image/png") {
		t.Fatalf("proof content-type = %q", userProof.Header.Get("Content-Type"))
	}

	adminList := client.get(t, "/v1/admin/payments?status=pending_review", adminCookies)
	assertStatus(t, adminList, http.StatusOK)
	if got := numberField(t, adminList.JSON, "total"); got != 1 {
		t.Fatalf("pending review total = %v", got)
	}
	adminItems := adminList.JSON["items"].([]any)
	if got := adminItems[0].(map[string]any)["proof_uploaded"]; got != true {
		t.Fatalf("proof_uploaded = %#v", got)
	}

	missingReference := client.post(t, "/v1/admin/payments/"+paymentID+"/approve", map[string]any{}, adminCookies)
	assertStatus(t, missingReference, http.StatusBadRequest)

	approved := client.post(t, "/v1/admin/payments/"+paymentID+"/approve", map[string]any{
		"payment_reference": " bk 123 456 ",
		"note":              "Verified in bank app",
	}, adminCookies)
	assertStatus(t, approved, http.StatusOK)
	if got := stringField(t, approved.JSON, "payment.status"); got != "paid" {
		t.Fatalf("approved status = %q", got)
	}
	if got := numberField(t, approved.JSON, "balance.availableUnits"); got != 1250000 {
		t.Fatalf("balance.availableUnits = %v", got)
	}

	doubleApprove := client.post(t, "/v1/admin/payments/"+paymentID+"/approve", map[string]any{"payment_reference": "BK123456"}, adminCookies)
	assertStatus(t, doubleApprove, http.StatusConflict)

	secondCheckout := client.post(t, "/v1/checkout/package/"+packageID, map[string]any{}, userCookies)
	assertStatus(t, secondCheckout, http.StatusCreated)
	secondPaymentID := stringField(t, secondCheckout.JSON, "payment.id")
	secondProof := client.postMultipart(t, "/v1/payments/"+secondPaymentID+"/proof", map[string]string{}, "proof2.png", "image/png", tinyPNG(), userCookies)
	assertStatus(t, secondProof, http.StatusOK)
	duplicateRef := client.post(t, "/v1/admin/payments/"+secondPaymentID+"/approve", map[string]any{"payment_reference": "BK123456"}, adminCookies)
	assertStatus(t, duplicateRef, http.StatusConflict)
	assertBalance(t, user.ID, 1250000)

	rejected := client.post(t, "/v1/admin/payments/"+secondPaymentID+"/reject", map[string]any{"reason": "Amount does not match"}, adminCookies)
	assertStatus(t, rejected, http.StatusOK)
	if got := stringField(t, rejected.JSON, "payment.status"); got != "rejected" {
		t.Fatalf("rejected status = %q", got)
	}
	assertBalance(t, user.ID, 1250000)

	var auditCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_logs WHERE admin_id = $1 AND action IN ('payment_settings_updated', 'payment_settings_khqr_uploaded', 'payment_approved', 'payment_rejected')`, adminUser.ID).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount < 4 {
		t.Fatalf("audit count = %d", auditCount)
	}

	for _, body := range []string{checkout.Body, proof.Body, adminList.Body, approved.Body} {
		if strings.Contains(body, "payment-proofs/") || strings.Contains(body, "khqr_image_path") || strings.Contains(body, "file_path") {
			t.Fatalf("response leaked storage path: %s", body)
		}
	}
}

func TestSLAIPaymentCheckoutAndSignedCallbackCreditsBalance(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	fakePayments := newFakeSLAIPaymentClient()
	client := newTestClientWithSLAIPayment(t, fakePayments, "callback-secret")
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "auto-pay-user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	providerStatus := client.get(t, "/v1/admin/payment-settings/bakong-khqr/provider-status", adminCookies)
	assertStatus(t, providerStatus, http.StatusOK)
	if got := boolField(t, providerStatus.JSON, "provider_status.enabled"); !got {
		t.Fatalf("provider status enabled = false")
	}
	if got := boolField(t, providerStatus.JSON, "provider_status.callback_secret_configured"); !got {
		t.Fatalf("provider callback secret configured = false")
	}

	pkgResp := client.post(t, "/v1/admin/packages", map[string]any{
		"name":             "Auto Starter",
		"description":      "Auto payment package",
		"creditUnits":      1000000,
		"bonusCreditUnits": 250000,
		"priceMinor":       100,
		"currency":         "USD",
		"active":           true,
	}, adminCookies)
	assertStatus(t, pkgResp, http.StatusCreated)
	packageID := stringField(t, pkgResp.JSON, "package.id")

	checkout := client.post(t, "/v1/checkout/package/"+packageID, map[string]any{}, userCookies)
	assertStatus(t, checkout, http.StatusCreated)
	paymentID := stringField(t, checkout.JSON, "payment.id")
	if got := stringField(t, checkout.JSON, "payment.status"); got != "pending_payment" {
		t.Fatalf("checkout status = %q", got)
	}
	if got := stringField(t, checkout.JSON, "checkout.qr_image_data_uri"); !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("checkout QR image missing: %q", got)
	}
	if len(fakePayments.created) != 1 || fakePayments.created[0].Amount != "1.00" {
		t.Fatalf("unexpected provider create request: %#v", fakePayments.created)
	}

	external := fakePayments.payments[fakePayments.created[0].Reference]
	paidAt := time.Now().UTC()
	external.Status = "PAID"
	external.PaidAt = &paidAt
	external.Telegram = &slaipayment.TelegramPayment{
		Amount:        "1.00",
		Currency:      "USD",
		PaidAt:        paidAt,
		MerchantName:  external.MerchantName,
		Reference:     external.Reference,
		TransactionID: "177754980382419",
		APV:           "340383",
	}
	callback := slaipayment.CallbackPayload{Event: "payment.paid", Payment: external}
	callbackResp := client.postSignedSLAIPaymentCallback(t, callback, "callback-secret")
	assertStatus(t, callbackResp, http.StatusOK)
	if got := numberField(t, callbackResp.JSON, "balance.availableUnits"); got != 1250000 {
		t.Fatalf("callback balance = %v", got)
	}
	assertBalance(t, user.ID, 1250000)

	duplicate := client.postSignedSLAIPaymentCallback(t, callback, "callback-secret")
	assertStatus(t, duplicate, http.StatusOK)
	assertBalance(t, user.ID, 1250000)

	invalid := client.postRaw(t, "/v1/payments/slai-payment/callback", []byte(`{"event":"payment.paid"}`), nil)
	assertStatus(t, invalid, http.StatusUnauthorized)

	payment := client.get(t, "/v1/payments/"+paymentID, userCookies)
	assertStatus(t, payment, http.StatusOK)
	if got := stringField(t, payment.JSON, "payment.providerTransactionId"); got != "177754980382419" {
		t.Fatalf("provider transaction id = %q", got)
	}
}

func TestUserPaymentsListScopedToSessionUser(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "payments-user@example.com", users.RoleUser)
	otherUser := createUser(t, "other-payments-user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	userTopup := client.post(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      user.ID,
		"amountMinor": 1200,
		"currency":    "USD",
		"creditUnits": 12000,
		"note":        "wire received",
	}, adminCookies)
	assertStatus(t, userTopup, http.StatusCreated)
	otherTopup := client.post(t, "/v1/admin/payments/manual-topup", map[string]any{
		"userId":      otherUser.ID,
		"amountMinor": 900,
		"currency":    "USD",
		"creditUnits": 9000,
	}, adminCookies)
	assertStatus(t, otherTopup, http.StatusCreated)

	payments := client.get(t, "/v1/payments", userCookies)
	assertStatus(t, payments, http.StatusOK)
	items := payments.JSON["payments"].([]any)
	if len(items) != 1 {
		t.Fatalf("payments len = %d, want 1 body=%s", len(items), payments.Body)
	}
	payment := items[0].(map[string]any)
	if got := stringField(t, payment, "userId"); got != user.ID {
		t.Fatalf("payment userId = %q, want %q", got, user.ID)
	}
	if got := numberField(t, payment, "amountMinor"); got != 1200 {
		t.Fatalf("payment amountMinor = %v, want 1200", got)
	}
	if strings.Contains(payments.Body, otherUser.ID) {
		t.Fatalf("payments list leaked another user payment: %s", payments.Body)
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

func TestUserUsageListFiltersPaginationAndScope(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	adminUser := createUser(t, "admin@example.com", users.RoleAdmin)
	user := createUser(t, "usage-user@example.com", users.RoleUser)
	otherUser := createUser(t, "other-usage-user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)
	otherCookies := loginCookies(t, client, otherUser.Email)

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, created, http.StatusCreated)
	keyID := stringField(t, created.JSON, "api_key.id")
	otherCreated := client.post(t, "/v1/api-key", map[string]any{"name": "Other"}, otherCookies)
	assertStatus(t, otherCreated, http.StatusCreated)
	otherKeyID := stringField(t, otherCreated.JSON, "api_key.id")

	for _, event := range []map[string]any{
		{
			"api_key_id":        keyID,
			"external_event_id": "user-usage-001",
			"model":             "gpt-5.5",
			"provider":          "openai",
			"input_tokens":      100,
			"output_tokens":     50,
			"occurred_at":       "2026-04-28T10:00:00Z",
		},
		{
			"api_key_id":        keyID,
			"external_event_id": "user-usage-002",
			"model":             "claude-sonnet",
			"provider":          "anthropic",
			"input_tokens":      200,
			"output_tokens":     75,
			"occurred_at":       "2026-04-29T10:00:00Z",
		},
		{
			"api_key_id":        otherKeyID,
			"external_event_id": "other-usage-001",
			"model":             "gpt-5.5",
			"provider":          "openai",
			"input_tokens":      300,
			"output_tokens":     125,
			"occurred_at":       "2026-04-29T11:00:00Z",
		},
	} {
		ingested := client.post(t, "/v1/internal/usage/mock-event", event, adminCookies)
		assertStatus(t, ingested, http.StatusCreated)
	}

	all := client.get(t, "/v1/usage", userCookies)
	assertStatus(t, all, http.StatusOK)
	allItems := all.JSON["usage"].([]any)
	if len(allItems) != 2 {
		t.Fatalf("usage len = %d, want 2 body=%s", len(allItems), all.Body)
	}

	modelFiltered := client.get(t, "/v1/usage?model=gpt-5.5", userCookies)
	assertStatus(t, modelFiltered, http.StatusOK)
	modelItems := modelFiltered.JSON["usage"].([]any)
	if len(modelItems) != 1 {
		t.Fatalf("model filtered len = %d, want 1 body=%s", len(modelItems), modelFiltered.Body)
	}
	if got := stringField(t, modelItems[0].(map[string]any), "external_event_id"); got != "user-usage-001" {
		t.Fatalf("model filtered event = %q", got)
	}

	providerFiltered := client.get(t, "/v1/usage?provider=anthropic", userCookies)
	assertStatus(t, providerFiltered, http.StatusOK)
	if got := stringField(t, providerFiltered.JSON["usage"].([]any)[0].(map[string]any), "external_event_id"); got != "user-usage-002" {
		t.Fatalf("provider filtered event = %q", got)
	}

	statusFiltered := client.get(t, "/v1/usage?status=billed", userCookies)
	assertStatus(t, statusFiltered, http.StatusOK)
	if got := len(statusFiltered.JSON["usage"].([]any)); got != 2 {
		t.Fatalf("status filtered len = %d, want 2", got)
	}

	timeFiltered := client.get(t, "/v1/usage?from=2026-04-29T00:00:00Z&to=2026-04-29T23:59:59Z", userCookies)
	assertStatus(t, timeFiltered, http.StatusOK)
	timeItems := timeFiltered.JSON["usage"].([]any)
	if len(timeItems) != 1 {
		t.Fatalf("time filtered len = %d, want 1 body=%s", len(timeItems), timeFiltered.Body)
	}
	if got := stringField(t, timeItems[0].(map[string]any), "external_event_id"); got != "user-usage-002" {
		t.Fatalf("time filtered event = %q", got)
	}

	firstPage := client.get(t, "/v1/usage?limit=1&offset=0", userCookies)
	assertStatus(t, firstPage, http.StatusOK)
	if got := stringField(t, firstPage.JSON["usage"].([]any)[0].(map[string]any), "external_event_id"); got != "user-usage-002" {
		t.Fatalf("first page event = %q", got)
	}
	secondPage := client.get(t, "/v1/usage?limit=1&offset=1", userCookies)
	assertStatus(t, secondPage, http.StatusOK)
	if got := stringField(t, secondPage.JSON["usage"].([]any)[0].(map[string]any), "external_event_id"); got != "user-usage-001" {
		t.Fatalf("second page event = %q", got)
	}

	invalidDate := client.get(t, "/v1/usage?from=not-a-date", userCookies)
	assertStatus(t, invalidDate, http.StatusBadRequest)
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
	statusBefore := before.JSON["sync_status"].(map[string]any)
	if got := statusBefore["omniroute_enabled"]; got != false {
		t.Fatalf("omniroute_enabled = %#v, want false", got)
	}
	if got := statusBefore["sync_mode"]; got != "call_logs" {
		t.Fatalf("sync_mode = %#v, want call_logs", got)
	}
	if got := statusBefore["worker_interval_seconds"]; got != float64(60) {
		t.Fatalf("worker_interval_seconds = %#v, want 60", got)
	}
	if got := statusBefore["batch_limit"]; got != float64(100) {
		t.Fatalf("batch_limit = %#v, want 100", got)
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
		OmniRoute: config.OmniRouteConfig{
			Enabled:       false,
			UsageSyncMode: "call_logs",
			CallLogLimit:  100,
		},
		UsageSyncWorker: config.UsageSyncWorkerConfig{
			Enabled:    false,
			Interval:   60 * time.Second,
			LockKey:    "slai_usage_sync",
			BatchLimit: 100,
			StartDelay: 10 * time.Second,
		},
		Storage: config.StorageConfig{
			Dir:               t.TempDir(),
			PaymentProofMaxMB: 5,
			PaymentQRMaxMB:    2,
		},
	}, testDB, slog.New(slog.NewTextHandler(io.Discard, nil)))

	return testClient{
		server: httptest.NewServer(server),
		client: &http.Client{},
	}
}

func newTestClientWithSLAIPayment(t *testing.T, paymentClient slaipayment.Client, callbackSecret string) testClient {
	t.Helper()
	server := httpserver.NewServer(httpserver.ServerConfig{
		Addr:             ":0",
		ReadinessTimeout: time.Second,
		SessionSecret:    "test-secret",
		CookieSecure:     false,
		SessionTTL:       time.Hour,
		APIKeyPepper:     "test-api-key-pepper",
		APIKeyPrefix:     "sk_slai",
		OmniRoute: config.OmniRouteConfig{
			Enabled:       false,
			UsageSyncMode: "call_logs",
			CallLogLimit:  100,
		},
		UsageSyncWorker: config.UsageSyncWorkerConfig{
			Enabled:    false,
			Interval:   60 * time.Second,
			LockKey:    "slai_usage_sync",
			BatchLimit: 100,
			StartDelay: 10 * time.Second,
		},
		Storage: config.StorageConfig{
			Dir:               t.TempDir(),
			PaymentProofMaxMB: 5,
			PaymentQRMaxMB:    2,
		},
		SLAIPayment: config.SLAIPaymentConfig{
			Enabled:         true,
			BaseURL:         "http://slai-payment.test",
			CallbackBaseURL: "http://slai-api.test",
			CallbackSecret:  callbackSecret,
			MerchantPrefix:  "SLAI",
			DefaultExpiry:   30 * time.Minute,
			HTTPTimeout:     time.Second,
		},
		SLAIPaymentClient: paymentClient,
	}, testDB, slog.New(slog.NewTextHandler(io.Discard, nil)))

	return testClient{
		server: httptest.NewServer(server),
		client: &http.Client{},
	}
}

type fakeSLAIPaymentClient struct {
	created  []slaipayment.CreatePaymentInput
	payments map[string]slaipayment.Payment
}

func newFakeSLAIPaymentClient() *fakeSLAIPaymentClient {
	return &fakeSLAIPaymentClient{payments: map[string]slaipayment.Payment{}}
}

func (c *fakeSLAIPaymentClient) CreatePayment(_ context.Context, input slaipayment.CreatePaymentInput) (slaipayment.Payment, error) {
	c.created = append(c.created, input)
	now := time.Now().UTC()
	payment := slaipayment.Payment{
		ID:             "pay_" + input.Reference,
		Reference:      input.Reference,
		MerchantPrefix: input.MerchantPrefix,
		MerchantName:   input.MerchantPrefix + " " + input.Reference,
		Amount:         input.Amount,
		Currency:       input.Currency,
		Status:         "PENDING",
		QRPayload:      "000201" + input.Reference,
		QRMD5:          "qr-md5-" + input.Reference,
		QRImageDataURI: "data:image/png;base64,AAAA",
		CreatedAt:      now,
		ExpiresAt:      now.Add(30 * time.Minute),
	}
	c.payments[input.Reference] = payment
	return payment, nil
}

func (c *fakeSLAIPaymentClient) GetPayment(_ context.Context, id string) (slaipayment.Payment, error) {
	for _, payment := range c.payments {
		if payment.ID == id {
			return payment, nil
		}
	}
	return slaipayment.Payment{}, slaipayment.ErrNotFound
}

func (c *fakeSLAIPaymentClient) GetPaymentByReference(_ context.Context, reference string) (slaipayment.Payment, error) {
	payment, ok := c.payments[reference]
	if !ok {
		return slaipayment.Payment{}, slaipayment.ErrNotFound
	}
	return payment, nil
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

func (c testClient) postSignedSLAIPaymentCallback(t *testing.T, payload slaipayment.CallbackPayload, secret string) testResponse {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return c.postRaw(t, "/v1/payments/slai-payment/callback", body, map[string]string{
		"Content-Type":             "application/json",
		"X-SLAI-Payment-Timestamp": timestamp,
		"X-SLAI-Payment-Signature": "v1=" + hex.EncodeToString(mac.Sum(nil)),
		"X-SLAI-Payment-ID":        payload.Payment.ID,
	})
}

func (c testClient) postRaw(t *testing.T, path string, body []byte, headers map[string]string) testResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.server.URL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	return c.do(t, req)
}

func (c testClient) postMultipart(t *testing.T, path string, fields map[string]string, fileName string, contentType string, data []byte, cookies []*http.Cookie) testResponse {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, fileName))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, c.server.URL+path, &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	return c.do(t, req)
}

func tinyPNG() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82}
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
		TRUNCATE admin_audit_logs, credit_ledger_entries, payment_proofs, payments, credit_balances, sessions, credit_packages, users
		RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = testDB.Exec(context.Background(), `
		INSERT INTO payment_settings (provider, display_name, enabled)
		VALUES ('bakong_khqr', 'Bakong KHQR', false)
		ON CONFLICT (provider) DO UPDATE
		SET enabled = false,
		    display_name = 'Bakong KHQR',
		    account_name = NULL,
		    account_id = NULL,
		    khqr_image_path = NULL,
		    khqr_image_mime = NULL,
		    instructions = NULL
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

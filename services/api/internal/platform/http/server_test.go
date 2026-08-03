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
	"github.com/slai/slai/services/api/internal/apikeys"
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
	emailSender := &captureEmailSender{otps: map[string]string{}}
	client := newTestClientWithEmailSender(t, emailSender)

	signup := client.post(t, "/v1/auth/signup", map[string]any{
		"email":    "Developer@Example.com",
		"password": "correct-password",
	}, nil)
	assertStatus(t, signup, http.StatusAccepted)
	if got := stringField(t, signup.JSON, "verification.email"); got != "developer@example.com" {
		t.Fatalf("email = %q", got)
	}
	if len(signup.Cookies()) != 0 {
		t.Fatal("did not expect a session cookie before email verification")
	}

	loginBeforeVerification := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "developer@example.com",
		"password": "correct-password",
	}, nil)
	assertStatus(t, loginBeforeVerification, http.StatusUnauthorized)

	badVerify := client.post(t, "/v1/auth/signup/verify", map[string]any{
		"email": "developer@example.com",
		"otp":   "000000",
	}, nil)
	assertStatus(t, badVerify, http.StatusBadRequest)

	verify := client.post(t, "/v1/auth/signup/verify", map[string]any{
		"email": "Developer@Example.com",
		"otp":   emailSender.otps["developer@example.com"],
	}, nil)
	assertStatus(t, verify, http.StatusCreated)
	if got := stringField(t, verify.JSON, "user.email"); got != "developer@example.com" {
		t.Fatalf("email = %q", got)
	}
	if len(verify.Cookies()) == 0 {
		t.Fatal("expected session cookie after verification")
	}

	me := client.get(t, "/v1/me", verify.Cookies())
	assertStatus(t, me, http.StatusOK)
	if got := stringField(t, me.JSON, "user.role"); got != users.RoleUser {
		t.Fatalf("role = %q", got)
	}

	logout := client.post(t, "/v1/auth/logout", map[string]any{}, verify.Cookies())
	assertStatus(t, logout, http.StatusOK)
	meAfterLogout := client.get(t, "/v1/me", verify.Cookies())
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

func TestSignupOTPRateLimitsResendAndIP(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	emailSender := &captureEmailSender{otps: map[string]string{}, counts: map[string]int{}}
	client := newTestClientWithEmailSenderAndEmailConfig(t, emailSender, config.EmailConfig{
		SignupOTPResendCooldown:   time.Hour,
		SignupOTPRequestWindow:    time.Hour,
		SignupOTPMaxEmailRequests: 3,
		SignupOTPMaxIPRequests:    2,
	})

	first := client.postWithHeaders(t, "/v1/auth/signup", map[string]any{
		"email":    "limited@example.com",
		"password": "correct-password",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.10"})
	assertStatus(t, first, http.StatusAccepted)

	resend := client.postWithHeaders(t, "/v1/auth/signup", map[string]any{
		"email":    "limited@example.com",
		"password": "correct-password",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.10"})
	assertStatus(t, resend, http.StatusTooManyRequests)
	if got := stringField(t, resend.JSON, "error"); got != "verification_code_requested_too_soon" {
		t.Fatalf("resend error = %q", got)
	}
	if retryAfter := resend.Header.Get("Retry-After"); retryAfter == "" {
		t.Fatal("expected Retry-After header")
	}
	if emailSender.counts["limited@example.com"] != 1 {
		t.Fatalf("limited@example.com email count = %d", emailSender.counts["limited@example.com"])
	}

	secondEmail := client.postWithHeaders(t, "/v1/auth/signup", map[string]any{
		"email":    "second@example.com",
		"password": "correct-password",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.10"})
	assertStatus(t, secondEmail, http.StatusAccepted)

	thirdEmail := client.postWithHeaders(t, "/v1/auth/signup", map[string]any{
		"email":    "third@example.com",
		"password": "correct-password",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.10"})
	assertStatus(t, thirdEmail, http.StatusTooManyRequests)
	if got := stringField(t, thirdEmail.JSON, "error"); got != "too_many_verification_code_requests" {
		t.Fatalf("ip limit error = %q", got)
	}
	if emailSender.counts["third@example.com"] != 0 {
		t.Fatalf("third@example.com email count = %d", emailSender.counts["third@example.com"])
	}
}

func TestPasswordResetOTPFlow(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	emailSender := &captureEmailSender{passwordResetOTPs: map[string]string{}, passwordResetCounts: map[string]int{}, passwordChangedCounts: map[string]int{}}
	client := newTestClientWithEmailSender(t, emailSender)
	user := createUser(t, "reset@example.com", users.RoleUser)
	existingCookies := loginCookies(t, client, user.Email)

	request := client.post(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "Reset@Example.com",
	}, nil)
	assertStatus(t, request, http.StatusAccepted)
	if got := stringField(t, request.JSON, "verification.email"); got != "reset@example.com" {
		t.Fatalf("email = %q", got)
	}
	if emailSender.passwordResetCounts["reset@example.com"] != 1 {
		t.Fatalf("reset email count = %d", emailSender.passwordResetCounts["reset@example.com"])
	}

	badConfirm := client.post(t, "/v1/auth/password-reset/confirm", map[string]any{
		"email":    "reset@example.com",
		"otp":      "000000",
		"password": "new-password",
	}, nil)
	assertStatus(t, badConfirm, http.StatusBadRequest)

	confirm := client.post(t, "/v1/auth/password-reset/confirm", map[string]any{
		"email":    "Reset@Example.com",
		"otp":      emailSender.passwordResetOTPs["reset@example.com"],
		"password": "new-password",
	}, nil)
	assertStatus(t, confirm, http.StatusOK)
	if emailSender.passwordChangedCounts["reset@example.com"] != 1 {
		t.Fatalf("password changed email count = %d", emailSender.passwordChangedCounts["reset@example.com"])
	}

	meWithOldSession := client.get(t, "/v1/me", existingCookies)
	assertStatus(t, meWithOldSession, http.StatusUnauthorized)

	oldLogin := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "reset@example.com",
		"password": "correct-password",
	}, nil)
	assertStatus(t, oldLogin, http.StatusUnauthorized)

	newLogin := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "reset@example.com",
		"password": "new-password",
	}, nil)
	assertStatus(t, newLogin, http.StatusOK)
}

func TestPasswordResetRequestDoesNotEnumerateUsers(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	emailSender := &captureEmailSender{passwordResetCounts: map[string]int{}}
	client := newTestClientWithEmailSender(t, emailSender)

	unknown := client.post(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "missing@example.com",
	}, nil)
	assertStatus(t, unknown, http.StatusAccepted)
	if emailSender.passwordResetCounts["missing@example.com"] != 0 {
		t.Fatalf("missing@example.com reset email count = %d", emailSender.passwordResetCounts["missing@example.com"])
	}

	createGoogleUser(t, "google@example.com", "google-subject")
	googleOnly := client.post(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "google@example.com",
	}, nil)
	assertStatus(t, googleOnly, http.StatusAccepted)
	if emailSender.passwordResetCounts["google@example.com"] != 0 {
		t.Fatalf("google@example.com reset email count = %d", emailSender.passwordResetCounts["google@example.com"])
	}
}

func TestPasswordResetOTPRateLimitsResendAndIP(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	emailSender := &captureEmailSender{passwordResetOTPs: map[string]string{}, passwordResetCounts: map[string]int{}}
	client := newTestClientWithEmailSenderAndEmailConfig(t, emailSender, config.EmailConfig{
		PasswordResetOTPResendCooldown:   time.Hour,
		PasswordResetOTPRequestWindow:    time.Hour,
		PasswordResetOTPMaxEmailRequests: 3,
		PasswordResetOTPMaxIPRequests:    2,
	})
	createUser(t, "limited@example.com", users.RoleUser)
	createUser(t, "second@example.com", users.RoleUser)
	createUser(t, "third@example.com", users.RoleUser)

	first := client.postWithHeaders(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "limited@example.com",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.20"})
	assertStatus(t, first, http.StatusAccepted)

	resend := client.postWithHeaders(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "limited@example.com",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.20"})
	assertStatus(t, resend, http.StatusTooManyRequests)
	if got := stringField(t, resend.JSON, "error"); got != "verification_code_requested_too_soon" {
		t.Fatalf("resend error = %q", got)
	}
	if retryAfter := resend.Header.Get("Retry-After"); retryAfter == "" {
		t.Fatal("expected Retry-After header")
	}
	if emailSender.passwordResetCounts["limited@example.com"] != 1 {
		t.Fatalf("limited@example.com reset email count = %d", emailSender.passwordResetCounts["limited@example.com"])
	}

	second := client.postWithHeaders(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "second@example.com",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.20"})
	assertStatus(t, second, http.StatusAccepted)

	third := client.postWithHeaders(t, "/v1/auth/password-reset/request", map[string]any{
		"email": "third@example.com",
	}, nil, map[string]string{"X-Forwarded-For": "203.0.113.20"})
	assertStatus(t, third, http.StatusTooManyRequests)
	if got := stringField(t, third.JSON, "error"); got != "too_many_verification_code_requests" {
		t.Fatalf("ip limit error = %q", got)
	}
	if emailSender.passwordResetCounts["third@example.com"] != 0 {
		t.Fatalf("third@example.com reset email count = %d", emailSender.passwordResetCounts["third@example.com"])
	}
}

func TestChangePasswordRevokesSessionsAndRequiresCurrentPassword(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	emailSender := &captureEmailSender{passwordChangedCounts: map[string]int{}}
	client := newTestClientWithEmailSender(t, emailSender)
	user := createUser(t, "security@example.com", users.RoleUser)
	firstSession := loginCookies(t, client, user.Email)
	secondSession := loginCookies(t, client, user.Email)

	wrongCurrent := client.post(t, "/v1/me/password", map[string]any{
		"currentPassword": "wrong-password",
		"newPassword":     "new-password",
	}, firstSession)
	assertStatus(t, wrongCurrent, http.StatusUnauthorized)
	if got := stringField(t, wrongCurrent.JSON, "error"); got != "invalid_current_password" {
		t.Fatalf("error = %q", got)
	}

	tooShort := client.post(t, "/v1/me/password", map[string]any{
		"currentPassword": "correct-password",
		"newPassword":     "short",
	}, firstSession)
	assertStatus(t, tooShort, http.StatusBadRequest)
	if got := stringField(t, tooShort.JSON, "error"); got != "password_change_failed" {
		t.Fatalf("error = %q", got)
	}

	changed := client.post(t, "/v1/me/password", map[string]any{
		"currentPassword": "correct-password",
		"newPassword":     "new-password",
	}, firstSession)
	assertStatus(t, changed, http.StatusOK)
	if emailSender.passwordChangedCounts["security@example.com"] != 1 {
		t.Fatalf("password changed email count = %d", emailSender.passwordChangedCounts["security@example.com"])
	}

	meWithFirstSession := client.get(t, "/v1/me", firstSession)
	assertStatus(t, meWithFirstSession, http.StatusUnauthorized)
	meWithSecondSession := client.get(t, "/v1/me", secondSession)
	assertStatus(t, meWithSecondSession, http.StatusUnauthorized)

	oldLogin := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "security@example.com",
		"password": "correct-password",
	}, nil)
	assertStatus(t, oldLogin, http.StatusUnauthorized)

	newLogin := client.post(t, "/v1/auth/login", map[string]any{
		"email":    "security@example.com",
		"password": "new-password",
	}, nil)
	assertStatus(t, newLogin, http.StatusOK)
}

func TestChangePasswordRejectsGoogleOnlyUser(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	verifier := &fakeGoogleVerifier{identities: map[string]auth.GoogleIdentity{
		"google-token": {
			Subject:       "google-subject-security",
			Email:         "google-security@example.com",
			EmailVerified: true,
		},
	}}
	client := newTestClientWithGoogle(t, verifier)
	login := client.post(t, "/v1/auth/google", map[string]any{"credential": "google-token"}, nil)
	assertStatus(t, login, http.StatusOK)

	response := client.post(t, "/v1/me/password", map[string]any{
		"currentPassword": "unused-password",
		"newPassword":     "new-password",
	}, login.Cookies())
	assertStatus(t, response, http.StatusConflict)
	if got := stringField(t, response.JSON, "error"); got != "password_auth_unavailable" {
		t.Fatalf("error = %q", got)
	}
}

func TestGoogleAuthCreatesLinksAndLogsInUser(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	verifier := &fakeGoogleVerifier{
		identities: map[string]auth.GoogleIdentity{
			"new-user-token": {
				Subject:       "google-subject-new",
				Email:         "GoogleUser@Example.com",
				EmailVerified: true,
			},
			"existing-user-token": {
				Subject:       "google-subject-existing",
				Email:         "developer@example.com",
				EmailVerified: true,
			},
		},
	}
	client := newTestClientWithGoogle(t, verifier)

	created := client.post(t, "/v1/auth/google", map[string]any{"credential": "new-user-token"}, nil)
	assertStatus(t, created, http.StatusOK)
	if got := stringField(t, created.JSON, "user.email"); got != "googleuser@example.com" {
		t.Fatalf("email = %q", got)
	}
	if got := stringField(t, created.JSON, "user.authProvider"); got != users.AuthProviderGoogle {
		t.Fatalf("authProvider = %q", got)
	}
	me := client.get(t, "/v1/me", created.Cookies())
	assertStatus(t, me, http.StatusOK)

	passwordUser := createUser(t, "developer@example.com", users.RoleUser)
	linked := client.post(t, "/v1/auth/google", map[string]any{"credential": "existing-user-token"}, nil)
	assertStatus(t, linked, http.StatusOK)
	if got := stringField(t, linked.JSON, "user.id"); got != passwordUser.ID {
		t.Fatalf("linked id = %q, want %q", got, passwordUser.ID)
	}
	if got := stringField(t, linked.JSON, "user.authProvider"); got != users.AuthProviderPassword {
		t.Fatalf("authProvider = %q", got)
	}
}

func TestGoogleAuthRejectsInvalidOrDisabledCredential(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	disabledClient := newTestClient(t)
	disabled := disabledClient.post(t, "/v1/auth/google", map[string]any{"credential": "token"}, nil)
	assertStatus(t, disabled, http.StatusServiceUnavailable)
	if got := stringField(t, disabled.JSON, "error"); got != "google_auth_not_configured" {
		t.Fatalf("error = %q", got)
	}

	client := newTestClientWithGoogle(t, &fakeGoogleVerifier{err: auth.ErrInvalidGoogleToken})
	invalid := client.post(t, "/v1/auth/google", map[string]any{"credential": "bad-token"}, nil)
	assertStatus(t, invalid, http.StatusUnauthorized)
	if got := stringField(t, invalid.JSON, "error"); got != "invalid_google_credential" {
		t.Fatalf("error = %q", got)
	}
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
	if got := item["auth_provider"]; got != users.AuthProviderPassword {
		t.Fatalf("auth_provider = %#v", got)
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
	if got := stringField(t, detail.JSON, "auth_provider"); got != users.AuthProviderPassword {
		t.Fatalf("auth_provider = %q", got)
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
	userCookies := loginCookies(t, client, user.Email)

	createdKey := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, createdKey, http.StatusCreated)
	suspendedKey := client.post(t, "/v1/admin/users/"+user.ID+"/api-key/suspend", map[string]any{}, adminCookies)
	assertStatus(t, suspendedKey, http.StatusOK)
	assertCurrentAPIKeyStatus(t, user.ID, apikeys.StatusSuspended)

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
	assertCurrentAPIKeyStatus(t, user.ID, apikeys.StatusActive)

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
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM admin_audit_logs WHERE admin_id = $1 AND action = 'manual_topup_created'`, adminUser.ID).Scan(&auditCount); err != nil {
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

	createdKey := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, userCookies)
	assertStatus(t, createdKey, http.StatusCreated)
	suspendedKey := client.post(t, "/v1/admin/users/"+user.ID+"/api-key/suspend", map[string]any{}, adminCookies)
	assertStatus(t, suspendedKey, http.StatusOK)
	assertCurrentAPIKeyStatus(t, user.ID, apikeys.StatusSuspended)

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
	checkoutExpiresAt := stringField(t, checkout.JSON, "payment.expiresAt")
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
	external.ExpiresAt = time.Time{}
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
	assertCurrentAPIKeyStatus(t, user.ID, apikeys.StatusActive)

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
	if got := stringField(t, payment.JSON, "payment.expiresAt"); got != checkoutExpiresAt {
		t.Fatalf("payment expiry = %q, want preserved checkout expiry %q", got, checkoutExpiresAt)
	}
}

func TestSLAIPaymentSignedExpiredCallbackMarksPaymentExpired(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	fakePayments := newFakeSLAIPaymentClient()
	client := newTestClientWithSLAIPayment(t, fakePayments, "callback-secret")
	adminUser := createUser(t, "expiry-admin@example.com", users.RoleAdmin)
	user := createUser(t, "expiry-user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	pkgResp := client.post(t, "/v1/admin/packages", map[string]any{
		"name":             "Expiry Starter",
		"description":      "Expiring payment package",
		"creditUnits":      2000000,
		"bonusCreditUnits": 0,
		"priceMinor":       250,
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

	external := fakePayments.payments[fakePayments.created[0].Reference]
	external.Status = "EXPIRED"
	external.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	callback := slaipayment.CallbackPayload{Event: "payment.expired", Payment: external}
	callbackResp := client.postSignedSLAIPaymentCallback(t, callback, "callback-secret")
	assertStatus(t, callbackResp, http.StatusOK)
	if got := stringField(t, callbackResp.JSON, "payment.status"); got != "expired" {
		t.Fatalf("expired callback payment status = %q", got)
	}
	assertBalance(t, user.ID, 0)

	var ledgerCount int
	if err := testDB.QueryRow(context.Background(), `SELECT count(*) FROM credit_ledger_entries WHERE user_id = $1`, user.ID).Scan(&ledgerCount); err != nil {
		t.Fatal(err)
	}
	if ledgerCount != 0 {
		t.Fatalf("ledger entries = %d, want 0", ledgerCount)
	}

	payment := client.get(t, "/v1/payments/"+paymentID, userCookies)
	assertStatus(t, payment, http.StatusOK)
	if got := stringField(t, payment.JSON, "payment.status"); got != "expired" {
		t.Fatalf("stored payment status = %q", got)
	}
	if got := stringField(t, payment.JSON, "payment.providerStatus"); got != "EXPIRED" {
		t.Fatalf("provider status = %q", got)
	}
}

func TestAdminCanResolveSLAIPaymentNeedsReview(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	fakePayments := newFakeSLAIPaymentClient()
	client := newTestClientWithSLAIPayment(t, fakePayments, "callback-secret")
	adminUser := createUser(t, "review-admin@example.com", users.RoleAdmin)
	user := createUser(t, "review-user@example.com", users.RoleUser)
	adminCookies := loginCookies(t, client, adminUser.Email)
	userCookies := loginCookies(t, client, user.Email)

	pkgResp := client.post(t, "/v1/admin/packages", map[string]any{
		"name":             "Review Starter",
		"description":      "Review package",
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
	external := fakePayments.payments[fakePayments.created[0].Reference]
	paidAt := time.Now().UTC()
	external.Status = "PAID"
	external.Amount = "2.00"
	external.PaidAt = &paidAt
	external.Telegram = &slaipayment.TelegramPayment{
		Amount:        "2.00",
		Currency:      "USD",
		PaidAt:        paidAt,
		MerchantName:  external.MerchantName,
		Reference:     external.Reference,
		TransactionID: "needs-review-tx-1",
		APV:           "needs-review-apv-1",
	}
	callback := client.postSignedSLAIPaymentCallback(t, slaipayment.CallbackPayload{Event: "payment.paid", Payment: external}, "callback-secret")
	assertStatus(t, callback, http.StatusOK)
	if got := stringField(t, callback.JSON, "payment.status"); got != "needs_review" {
		t.Fatalf("callback status = %q", got)
	}
	assertBalance(t, user.ID, 0)

	reviewQueue := client.get(t, "/v1/admin/payments?status=review_queue", adminCookies)
	assertStatus(t, reviewQueue, http.StatusOK)
	if got := numberField(t, reviewQueue.JSON, "total"); got != 1 {
		t.Fatalf("review queue total = %v", got)
	}

	approved := client.post(t, "/v1/admin/payments/"+paymentID+"/approve", map[string]any{
		"payment_reference": "needs-review-tx-1",
		"note":              "Verified mismatch manually",
	}, adminCookies)
	assertStatus(t, approved, http.StatusOK)
	if got := stringField(t, approved.JSON, "payment.status"); got != "paid" {
		t.Fatalf("approved status = %q", got)
	}
	if got := numberField(t, approved.JSON, "balance.availableUnits"); got != 1250000 {
		t.Fatalf("balance.availableUnits = %v", got)
	}

	secondCheckout := client.post(t, "/v1/checkout/package/"+packageID, map[string]any{}, userCookies)
	assertStatus(t, secondCheckout, http.StatusCreated)
	secondPaymentID := stringField(t, secondCheckout.JSON, "payment.id")
	secondExternal := fakePayments.payments[fakePayments.created[1].Reference]
	secondExternal.Status = "PAID"
	secondExternal.Amount = "2.00"
	secondExternal.PaidAt = &paidAt
	secondExternal.Telegram = &slaipayment.TelegramPayment{
		Amount:        "2.00",
		Currency:      "USD",
		PaidAt:        paidAt,
		MerchantName:  secondExternal.MerchantName,
		Reference:     secondExternal.Reference,
		TransactionID: "needs-review-tx-2",
		APV:           "needs-review-apv-2",
	}
	secondCallback := client.postSignedSLAIPaymentCallback(t, slaipayment.CallbackPayload{Event: "payment.paid", Payment: secondExternal}, "callback-secret")
	assertStatus(t, secondCallback, http.StatusOK)
	if got := stringField(t, secondCallback.JSON, "payment.status"); got != "needs_review" {
		t.Fatalf("second callback status = %q", got)
	}

	rejected := client.post(t, "/v1/admin/payments/"+secondPaymentID+"/reject", map[string]any{"reason": "Amount does not match checkout"}, adminCookies)
	assertStatus(t, rejected, http.StatusOK)
	if got := stringField(t, rejected.JSON, "payment.status"); got != "rejected" {
		t.Fatalf("rejected status = %q", got)
	}
	assertBalance(t, user.ID, 1250000)
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

func TestBalanceAcceptsSessionOrAPIKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	user := createUser(t, "balance@example.com", users.RoleUser)
	cookies := loginCookies(t, client, user.Email)

	sessionResponse := client.get(t, "/v1/balance", cookies)
	assertStatus(t, sessionResponse, http.StatusOK)
	if got := stringField(t, sessionResponse.JSON, "balance.userId"); got != user.ID {
		t.Fatalf("session balance user = %q, want %q", got, user.ID)
	}

	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, cookies)
	assertStatus(t, created, http.StatusCreated)
	rawKey := stringField(t, created.JSON, "raw_api_key")

	bearerResponse := client.getWithHeaders(t, "/v1/balance", nil, map[string]string{
		"Authorization": "Bearer " + rawKey,
	})
	assertStatus(t, bearerResponse, http.StatusOK)
	if got := stringField(t, bearerResponse.JSON, "balance.userId"); got != user.ID {
		t.Fatalf("bearer balance user = %q, want %q", got, user.ID)
	}
	for _, path := range []string{
		"balance.availableUnits",
		"balance.lifetimePurchasedUnits",
		"balance.lifetimeUsedUnits",
		"balance.version",
	} {
		_ = numberField(t, bearerResponse.JSON, path)
	}
	if got := stringField(t, bearerResponse.JSON, "balance.updatedAt"); got == "" {
		t.Fatal("balance.updatedAt is empty")
	}
}

func TestBalanceAcceptsSuspendedAPIKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	user := createUser(t, "suspended-key@example.com", users.RoleUser)
	cookies := loginCookies(t, client, user.Email)
	created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, cookies)
	assertStatus(t, created, http.StatusCreated)
	rawKey := stringField(t, created.JSON, "raw_api_key")

	service := apikeys.NewService(testDB, apikeys.Config{
		Pepper: "test-api-key-pepper",
		Prefix: "sk_slai",
	}, nil, slog.Default())
	if _, err := service.SuspendAPIKey(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}

	response := client.getWithHeaders(t, "/v1/balance", nil, map[string]string{
		"Authorization": "Bearer " + rawKey,
	})
	assertStatus(t, response, http.StatusOK)
	if got := stringField(t, response.JSON, "balance.userId"); got != user.ID {
		t.Fatalf("balance user = %q, want %q", got, user.ID)
	}
}

func TestBalanceRejectsInvalidAPIKeyWithoutCookieFallback(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClient(t)
	user := createUser(t, "cookie-owner@example.com", users.RoleUser)
	cookies := loginCookies(t, client, user.Email)

	for _, authorization := range []string{"", "Basic invalid", "Bearer sk_slai_unknown"} {
		response := client.getWithHeaders(t, "/v1/balance", cookies, map[string]string{
			"Authorization": authorization,
		})
		assertStatus(t, response, http.StatusUnauthorized)
		if got := stringField(t, response.JSON, "error"); got != "unauthenticated" {
			t.Fatalf("error = %q, want unauthenticated", got)
		}
	}
}

func TestBalanceRejectsRevokedKeyAndSuspendedUser(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T, service apikeys.Service, user users.User)
	}{
		{
			name: "revoked key",
			configure: func(t *testing.T, service apikeys.Service, user users.User) {
				t.Helper()
				if _, err := service.RevokeAPIKey(context.Background(), user.ID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "suspended user",
			configure: func(t *testing.T, _ apikeys.Service, user users.User) {
				t.Helper()
				if _, err := testDB.Exec(context.Background(), `UPDATE users SET status = $2 WHERE id = $1`, user.ID, users.StatusSuspended); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireDB(t)
			truncateTables(t)
			client := newTestClient(t)
			user := createUser(t, strings.ReplaceAll(tt.name, " ", "-")+"@example.com", users.RoleUser)
			cookies := loginCookies(t, client, user.Email)
			created := client.post(t, "/v1/api-key", map[string]any{"name": "Default"}, cookies)
			assertStatus(t, created, http.StatusCreated)
			rawKey := stringField(t, created.JSON, "raw_api_key")
			service := apikeys.NewService(testDB, apikeys.Config{
				Pepper: "test-api-key-pepper",
				Prefix: "sk_slai",
			}, nil, slog.Default())
			tt.configure(t, service, user)

			response := client.getWithHeaders(t, "/v1/balance", nil, map[string]string{
				"Authorization": "Bearer " + rawKey,
			})
			assertStatus(t, response, http.StatusUnauthorized)
			if got := stringField(t, response.JSON, "error"); got != "unauthenticated" {
				t.Fatalf("error = %q, want unauthenticated", got)
			}
		})
	}
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

func TestProvisionOmniRouteAPIKeyForExistingRawKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClientWithOmniRoute(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	service := apikeys.NewService(testDB, apikeys.Config{Pepper: "test-api-key-pepper", Prefix: "sk_slai", OmniRouteEnabled: false}, nil, slog.Default())
	created, err := service.CreateAPIKey(context.Background(), user.ID, "Legacy key")
	if err != nil {
		t.Fatal(err)
	}

	response := client.postWithHeaders(t, "/v1/internal/omniroute/api-keys/provision", map[string]any{
		"raw_api_key": created.RawAPIKey,
	}, nil, map[string]string{"Authorization": "Bearer omniroute-management-token"})
	assertStatus(t, response, http.StatusOK)

	omniRouteKeyID := stringField(t, response.JSON, "api_key.omniroute_key_id")
	if omniRouteKeyID != "slai-"+created.APIKey.ID {
		t.Fatalf("omniroute key id = %q", omniRouteKeyID)
	}
	if strings.Contains(response.Body, created.RawAPIKey) || strings.Contains(response.Body, "key_hash") {
		t.Fatalf("provision response leaked secret fields: %s", response.Body)
	}

	var storedOmniRouteKeyID string
	if err := testDB.QueryRow(context.Background(), `SELECT omniroute_key_id FROM api_keys WHERE id = $1`, created.APIKey.ID).Scan(&storedOmniRouteKeyID); err != nil {
		t.Fatal(err)
	}
	if storedOmniRouteKeyID != omniRouteKeyID {
		t.Fatalf("stored omniroute key id = %q", storedOmniRouteKeyID)
	}
}

func TestProvisionOmniRouteAPIKeyRejectsInvalidRawKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClientWithOmniRoute(t)

	response := client.postWithHeaders(t, "/v1/internal/omniroute/api-keys/provision", map[string]any{
		"raw_api_key": "sk_slai_missing",
	}, nil, map[string]string{"Authorization": "Bearer omniroute-management-token"})
	assertStatus(t, response, http.StatusNotFound)
}

func TestProvisionOmniRouteAPIKeyRequiresManagementToken(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	client := newTestClientWithOmniRoute(t)

	missing := client.post(t, "/v1/internal/omniroute/api-keys/provision", map[string]any{"raw_api_key": "sk_slai_missing"}, nil)
	assertStatus(t, missing, http.StatusUnauthorized)

	wrong := client.postWithHeaders(t, "/v1/internal/omniroute/api-keys/provision", map[string]any{"raw_api_key": "sk_slai_missing"}, nil, map[string]string{"Authorization": "Bearer wrong"})
	assertStatus(t, wrong, http.StatusForbidden)
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

func newTestClientWithEmailSender(t *testing.T, sender auth.EmailSender) testClient {
	return newTestClientWithEmailSenderAndEmailConfig(t, sender, config.EmailConfig{})
}

func newTestClientWithEmailSenderAndEmailConfig(t *testing.T, sender auth.EmailSender, emailCfg config.EmailConfig) testClient {
	t.Helper()
	if emailCfg.LowBalanceAlertThresholdUnits == 0 {
		emailCfg.LowBalanceAlertThresholdUnits = 5_000_000
	}
	emailCfg.PasswordChangedAlertsEnabled = true
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
		Email:       emailCfg,
		EmailSender: sender,
	}, testDB, slog.New(slog.NewTextHandler(io.Discard, nil)))

	return testClient{
		server: httptest.NewServer(server),
		client: &http.Client{},
	}
}

func newTestClientWithOmniRoute(t *testing.T) testClient {
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
			Enabled:         true,
			ManagementToken: "omniroute-management-token",
			UsageSyncMode:   "call_logs",
			CallLogLimit:    100,
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

func newTestClientWithGoogle(t *testing.T, verifier auth.GoogleIdentityVerifier) testClient {
	t.Helper()
	server := httpserver.NewServer(httpserver.ServerConfig{
		Addr:             ":0",
		ReadinessTimeout: time.Second,
		SessionSecret:    "test-secret",
		CookieSecure:     false,
		SessionTTL:       time.Hour,
		GoogleVerifier:   verifier,
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

type fakeGoogleVerifier struct {
	identities map[string]auth.GoogleIdentity
	err        error
}

type captureEmailSender struct {
	otps                    map[string]string
	counts                  map[string]int
	passwordResetOTPs       map[string]string
	passwordResetCounts     map[string]int
	passwordChangedCounts   map[string]int
	lowBalanceCounts        map[string]int
	lastLowBalanceUnits     int64
	lastLowBalanceThreshold int64
}

func (s *captureEmailSender) SendSignupOTP(_ context.Context, email, otp string, _ time.Time) error {
	if s.otps == nil {
		s.otps = map[string]string{}
	}
	if s.counts == nil {
		s.counts = map[string]int{}
	}
	s.otps[email] = otp
	s.counts[email]++
	return nil
}

func (s *captureEmailSender) SendPasswordResetOTP(_ context.Context, email, otp string, _ time.Time) error {
	if s.passwordResetOTPs == nil {
		s.passwordResetOTPs = map[string]string{}
	}
	if s.passwordResetCounts == nil {
		s.passwordResetCounts = map[string]int{}
	}
	s.passwordResetOTPs[email] = otp
	s.passwordResetCounts[email]++
	return nil
}

func (s *captureEmailSender) SendPasswordChangedAlert(_ context.Context, email string, _ time.Time) error {
	if s.passwordChangedCounts == nil {
		s.passwordChangedCounts = map[string]int{}
	}
	s.passwordChangedCounts[email]++
	return nil
}

func (s *captureEmailSender) SendLowBalanceAlert(_ context.Context, email string, balanceUnits, thresholdUnits int64) error {
	if s.lowBalanceCounts == nil {
		s.lowBalanceCounts = map[string]int{}
	}
	s.lowBalanceCounts[email]++
	s.lastLowBalanceUnits = balanceUnits
	s.lastLowBalanceThreshold = thresholdUnits
	return nil
}

func (v *fakeGoogleVerifier) VerifyGoogleIDToken(_ context.Context, credential string) (auth.GoogleIdentity, error) {
	if v.err != nil {
		return auth.GoogleIdentity{}, v.err
	}
	identity, ok := v.identities[credential]
	if !ok {
		return auth.GoogleIdentity{}, auth.ErrInvalidGoogleToken
	}
	return identity, nil
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
	return c.getWithHeaders(t, path, cookies, nil)
}

func (c testClient) getWithHeaders(t *testing.T, path string, cookies []*http.Cookie, headers map[string]string) testResponse {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.server.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
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

func createGoogleUser(t *testing.T, email, googleSubject string) users.User {
	t.Helper()
	var created users.User
	err := platformdb.InTx(context.Background(), testDB, func(tx pgx.Tx) error {
		user, err := users.NewRepository(tx).CreateGoogle(context.Background(), email, googleSubject, users.RoleUser)
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
		TRUNCATE admin_audit_logs, user_email_notifications, credit_ledger_entries, payment_proofs, payments, credit_balances, sessions, password_reset_otp_rate_limits, password_reset_otps, signup_otp_rate_limits, signup_email_verifications, credit_packages, users
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

func assertCurrentAPIKeyStatus(t *testing.T, userID string, expected string) {
	t.Helper()
	var actual string
	if err := testDB.QueryRow(context.Background(), `
		SELECT status
		FROM api_keys
		WHERE user_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT 1
	`, userID).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("api key status = %q, want %q", actual, expected)
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

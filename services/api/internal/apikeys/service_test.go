package apikeys_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	"github.com/slai/slai/services/api/internal/ledger"
	"github.com/slai/slai/services/api/internal/omniroute"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
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
	if err := platformdb.NewMigrator(pool, migrationsDir, slog.Default()).Up(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "run migrations: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func TestCreateAPIKeyStoresHashAndPrefixOnly(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	service := localService()

	created, err := service.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if created.RawAPIKey == "" {
		t.Fatal("raw api key was not returned on create")
	}
	if !strings.HasPrefix(created.RawAPIKey, "sk_slai_") {
		t.Fatalf("unexpected raw key prefix: %s", created.RawAPIKey)
	}

	var keyHash, keyPrefix string
	if err := testDB.QueryRow(context.Background(), `SELECT key_hash, key_prefix FROM api_keys WHERE user_id = $1`, user.ID).Scan(&keyHash, &keyPrefix); err != nil {
		t.Fatal(err)
	}
	if keyHash == created.RawAPIKey || keyPrefix == created.RawAPIKey {
		t.Fatal("raw api key was stored")
	}
	if !strings.HasPrefix(created.RawAPIKey, keyPrefix) {
		t.Fatalf("stored prefix %q is not a display prefix of raw key", keyPrefix)
	}
}

func TestCannotCreateSecondActiveAPIKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	service := localService()

	if _, err := service.CreateAPIKey(context.Background(), user.ID, "Default"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAPIKey(context.Background(), user.ID, "Second"); err == nil || !strings.Contains(err.Error(), apikeys.ErrActiveKeyExists.Error()) {
		t.Fatalf("expected active key exists error, got %v", err)
	}
}

func TestRotateRevokesOldKeyAndCreatesNewKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	service := localService()
	created, err := service.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := service.RotateAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.RawAPIKey == created.RawAPIKey {
		t.Fatal("rotation returned the old raw key")
	}

	var oldStatus, newStatus string
	if err := testDB.QueryRow(context.Background(), `SELECT status FROM api_keys WHERE id = $1`, created.APIKey.ID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if err := testDB.QueryRow(context.Background(), `SELECT status FROM api_keys WHERE id = $1`, rotated.APIKey.ID).Scan(&newStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != apikeys.StatusRevoked || newStatus != apikeys.StatusActive {
		t.Fatalf("statuses old=%s new=%s", oldStatus, newStatus)
	}
}

func TestRevokeSuspendAndResumeRules(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	service := localService()
	if _, err := service.CreateAPIKey(context.Background(), user.ID, "Default"); err != nil {
		t.Fatal(err)
	}

	suspended, err := service.SuspendAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if suspended.Status != apikeys.StatusSuspended {
		t.Fatalf("suspended status = %s", suspended.Status)
	}
	if _, err := service.ResumeAPIKey(context.Background(), user.ID); err == nil || !strings.Contains(err.Error(), apikeys.ErrInsufficientBalance.Error()) {
		t.Fatalf("expected insufficient balance, got %v", err)
	}

	creditUser(t, user.ID, 100)
	resumed, err := service.ResumeAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != apikeys.StatusActive {
		t.Fatalf("resumed status = %s", resumed.Status)
	}

	revoked, err := service.RevokeAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != apikeys.StatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("revoke result status=%s revoked_at=%v", revoked.Status, revoked.RevokedAt)
	}
}

func TestOmniRouteCallsWhenEnabled(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	mock := &mockOmniRoute{}
	service := apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: true}, mock, slog.Default())

	created, err := service.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if created.RawAPIKey != "omni_raw_1_abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("raw key = %s", created.RawAPIKey)
	}
	if mock.createCalls != 1 {
		t.Fatalf("create calls = %d", mock.createCalls)
	}

	if _, err := service.SuspendAPIKey(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	creditUser(t, user.ID, 100)
	if _, err := service.ResumeAPIKey(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	if mock.updateCalls != 2 {
		t.Fatalf("update calls = %d", mock.updateCalls)
	}
}

func TestRotateIgnoresMissingOmniRouteOldKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	mock := &mockOmniRoute{}
	service := apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: true}, mock, slog.Default())

	created, err := service.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}
	mock.deleteErr = omniroute.ErrNotFound

	rotated, err := service.RotateAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.APIKey.ID == created.APIKey.ID {
		t.Fatal("rotation did not create a replacement key")
	}
	if mock.deleteCalls != 1 || mock.createCalls != 2 {
		t.Fatalf("calls create=%d delete=%d", mock.createCalls, mock.deleteCalls)
	}

	var oldStatus, newStatus string
	if err := testDB.QueryRow(context.Background(), `SELECT status FROM api_keys WHERE id = $1`, created.APIKey.ID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if err := testDB.QueryRow(context.Background(), `SELECT status FROM api_keys WHERE id = $1`, rotated.APIKey.ID).Scan(&newStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != apikeys.StatusRevoked || newStatus != apikeys.StatusActive {
		t.Fatalf("statuses old=%s new=%s", oldStatus, newStatus)
	}
}

func TestRevokeIgnoresMissingOmniRouteKey(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	mock := &mockOmniRoute{deleteErr: omniroute.ErrNotFound}
	service := apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: true}, mock, slog.Default())

	if _, err := service.CreateAPIKey(context.Background(), user.ID, "Default"); err != nil {
		t.Fatal(err)
	}
	revoked, err := service.RevokeAPIKey(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != apikeys.StatusRevoked || revoked.RevokedAt == nil {
		t.Fatalf("revoke result status=%s revoked_at=%v", revoked.Status, revoked.RevokedAt)
	}
	if mock.deleteCalls != 1 {
		t.Fatalf("delete calls = %d", mock.deleteCalls)
	}
}

func TestDeleteOmniRouteKeyStillReturnsUnexpectedError(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "user@example.com", users.RoleUser)
	mock := &mockOmniRoute{deleteErr: errors.New("omniroute unavailable")}
	service := apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: true}, mock, slog.Default())

	if _, err := service.CreateAPIKey(context.Background(), user.ID, "Default"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RevokeAPIKey(context.Background(), user.ID); err == nil || !strings.Contains(err.Error(), "omniroute unavailable") {
		t.Fatalf("expected unexpected delete error, got %v", err)
	}
}

func localService() apikeys.Service {
	return apikeys.NewService(testDB, apikeys.Config{Pepper: "pepper", Prefix: "sk_slai", OmniRouteEnabled: false}, nil, slog.Default())
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

func truncateTables(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(), `
		TRUNCATE admin_audit_logs, credit_ledger_entries, payments, credit_balances, sessions, signup_otp_rate_limits, signup_email_verifications, credit_packages, api_keys, users
		RESTART IDENTITY CASCADE
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

type mockOmniRoute struct {
	mu          sync.Mutex
	createCalls int
	updateCalls int
	deleteCalls int
	deleteErr   error
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

func (m *mockOmniRoute) UpdateAPIKey(_ context.Context, _ string, _ omniroute.UpdateAPIKeyPayload) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCalls++
	return nil
}

func (m *mockOmniRoute) DeleteAPIKey(_ context.Context, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleteCalls++
	return m.deleteErr
}

func (m *mockOmniRoute) ListAPIKeys(context.Context) ([]omniroute.APIKey, error) { return nil, nil }
func (m *mockOmniRoute) FetchCallLogs(context.Context, *time.Time, int) ([]omniroute.CallLog, error) {
	return nil, nil
}
func (m *mockOmniRoute) FetchUsageHistory(context.Context, *time.Time, int) ([]omniroute.UsageRecord, error) {
	return nil, nil
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

	name := fmt.Sprintf("slai-apikeys-test-%d", time.Now().UnixNano())
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

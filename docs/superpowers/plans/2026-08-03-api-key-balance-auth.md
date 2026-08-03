# API-Key Balance Authentication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `GET /v1/balance` authenticate a user through either the existing session cookie or their SLAI API key.

**Architecture:** Add one read-only API-key service method that hashes a presented raw key and resolves its active user owner when the key is active or suspended. Add a balance-specific HTTP authentication helper that gives an explicit Authorization header precedence over cookies and otherwise preserves the existing session path; keep the balance response unchanged.

**Tech Stack:** Go 1.24, `net/http`, pgx/PostgreSQL, existing HMAC-SHA256 API-key hashing, Go integration tests, Markdown documentation.

## Global Constraints

- Change authentication only for `GET /v1/balance`; all other account endpoints remain session-authenticated.
- Accept API keys in `ACTIVE` or `SUSPENDED` status only when the owning user is `ACTIVE`.
- Reject revoked, unknown, and malformed credentials with generic `401 unauthenticated` responses.
- When an Authorization header is present, never fall back to a session cookie.
- Keep the successful balance JSON schema and integer micro-unit representation unchanged.
- Never persist, return, or log a presented raw API key.
- Do not update `api_keys.last_used_at` when reading a balance.
- Add no database migration or new dependency.

## File Structure

- Modify `services/api/internal/apikeys/service.go`: resolve a balance-readable API key to its owning user ID.
- Modify `services/api/internal/apikeys/service_test.go`: verify key and user status authentication rules.
- Modify `services/api/internal/platform/http/server.go`: select Bearer or session authentication for the balance handler.
- Modify `services/api/internal/platform/http/server_test.go`: verify the HTTP contract and credential precedence.
- Modify `README.md`: document Bearer authentication and provide a balance request example.

---

### Task 1: Resolve a Balance Owner from an API Key

**Files:**
- Modify: `services/api/internal/apikeys/service.go:58-66`
- Modify: `services/api/internal/apikeys/service_test.go:47-130`

**Interfaces:**
- Consumes: `HashRawKey(rawKey, s.cfg.Pepper) string`, `StatusActive`, `StatusSuspended`, `users.StatusActive`, and `ErrNotFound`.
- Produces: `func (s Service) AuthenticateBalanceKey(ctx context.Context, rawKey string) (string, error)`, returning the owning user ID for a permitted key.

- [ ] **Step 1: Write failing service tests for accepted key states**

Add these tests after `TestCreateAPIKeyStoresHashAndPrefixOnly`:

```go
func TestAuthenticateBalanceKeyAcceptsActiveAndSuspendedKeys(t *testing.T) {
	requireDB(t)
	truncateTables(t)
	user := createUser(t, "balance-owner@example.com", users.RoleUser)
	service := localService()
	created, err := service.CreateAPIKey(context.Background(), user.ID, "Default")
	if err != nil {
		t.Fatal(err)
	}

	userID, err := service.AuthenticateBalanceKey(context.Background(), created.RawAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if userID != user.ID {
		t.Fatalf("user id = %q, want %q", userID, user.ID)
	}

	if _, err := service.SuspendAPIKey(context.Background(), user.ID); err != nil {
		t.Fatal(err)
	}
	userID, err = service.AuthenticateBalanceKey(context.Background(), created.RawAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	if userID != user.ID {
		t.Fatalf("suspended key user id = %q, want %q", userID, user.ID)
	}
}
```

- [ ] **Step 2: Write failing service tests for rejected key and user states**

Add the following test beside the accepted-state test:

```go
func TestAuthenticateBalanceKeyRejectsUnusableCredentials(t *testing.T) {
	tests := []struct {
		name      string
		configure func(t *testing.T, service apikeys.Service, user users.User)
		rawKey    func(created apikeys.CreatedAPIKey) string
	}{
		{
			name: "unknown key",
			rawKey: func(apikeys.CreatedAPIKey) string {
				return "sk_slai_unknown"
			},
		},
		{
			name: "blank key",
			rawKey: func(apikeys.CreatedAPIKey) string {
				return "   "
			},
		},
		{
			name: "revoked key",
			configure: func(t *testing.T, service apikeys.Service, user users.User) {
				t.Helper()
				if _, err := service.RevokeAPIKey(context.Background(), user.ID); err != nil {
					t.Fatal(err)
				}
			},
			rawKey: func(created apikeys.CreatedAPIKey) string {
				return created.RawAPIKey
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
			rawKey: func(created apikeys.CreatedAPIKey) string {
				return created.RawAPIKey
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			truncateTables(t)
			user := createUser(t, strings.ReplaceAll(tt.name, " ", "-")+"@example.com", users.RoleUser)
			service := localService()
			created, err := service.CreateAPIKey(context.Background(), user.ID, "Default")
			if err != nil {
				t.Fatal(err)
			}
			if tt.configure != nil {
				tt.configure(t, service, user)
			}

			if _, err := service.AuthenticateBalanceKey(context.Background(), tt.rawKey(created)); !errors.Is(err, apikeys.ErrNotFound) {
				t.Fatalf("error = %v, want ErrNotFound", err)
			}
		})
	}
}
```

- [ ] **Step 3: Run the focused tests and confirm the missing-method failure**

Run:

```bash
cd services/api
go test ./internal/apikeys -run TestAuthenticateBalanceKey -count=1
```

Expected: compilation fails because `Service.AuthenticateBalanceKey` does not exist.

- [ ] **Step 4: Implement the minimal read-only key resolver**

Add this method after `GetLatestAPIKey` in `services/api/internal/apikeys/service.go`:

```go
func (s Service) AuthenticateBalanceKey(ctx context.Context, rawKey string) (string, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return "", ErrNotFound
	}

	var userID string
	err := s.pool.QueryRow(ctx, `
		SELECT ak.user_id::text
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.key_hash = $1
		  AND ak.status IN ($2, $3)
		  AND u.status = $4
	`, HashRawKey(rawKey, s.cfg.Pepper), StatusActive, StatusSuspended, users.StatusActive).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("authenticate balance api key: %w", err)
	}
	return userID, nil
}
```

- [ ] **Step 5: Format and run API-key service tests**

Run:

```bash
cd services/api
gofmt -w internal/apikeys/service.go internal/apikeys/service_test.go
go test ./internal/apikeys -count=1
```

Expected: all `internal/apikeys` tests pass. If Docker is unavailable, the package must compile and report PostgreSQL-dependent tests as skipped rather than failed.

- [ ] **Step 6: Commit the service boundary**

```bash
git add services/api/internal/apikeys/service.go services/api/internal/apikeys/service_test.go
git commit -m "feat: authenticate balance API keys"
```

---

### Task 2: Add Dual Authentication to `GET /v1/balance`

**Files:**
- Modify: `services/api/internal/platform/http/server.go:627-638,1499-1511`
- Modify: `services/api/internal/platform/http/server_test.go:1421-1470,2195-2205`
- Modify: `README.md:580-603`

**Interfaces:**
- Consumes: `apikeys.Service.AuthenticateBalanceKey(ctx context.Context, rawKey string) (string, error)` from Task 1 plus the existing `bearerToken` and `requireUser` helpers.
- Produces: `func (s *Server) requireBalanceUserID(w http.ResponseWriter, r *http.Request) (string, bool)` and dual authentication on the unchanged balance route.

- [ ] **Step 1: Add a GET test helper that accepts headers**

Replace the existing `testClient.get` helper with:

```go
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
```

- [ ] **Step 2: Write failing HTTP tests for session and Bearer success**

Add these tests before `TestPreventBalanceMutationOutsideLedgerService`:

```go
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
```

- [ ] **Step 3: Write failing HTTP tests for rejection and credential precedence**

Add these tests beside the success tests:

```go
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
```

- [ ] **Step 4: Run the focused HTTP tests and confirm Bearer authentication fails**

Run:

```bash
cd services/api
go test ./internal/platform/http -run 'TestBalance(Accepts|Rejects)' -count=1
```

Expected: session assertions pass, while Bearer success tests receive `401` because the handler still requires a cookie.

- [ ] **Step 5: Implement deterministic dual authentication**

Replace `Server.balance` with:

```go
func (s *Server) balance(w http.ResponseWriter, r *http.Request) {
	userID, ok := s.requireBalanceUserID(w, r)
	if !ok {
		return
	}
	balance, err := ledger.NewService(s.db).GetBalance(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "balance_failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"balance": balance})
}
```

Add this helper immediately after `requireUser`:

```go
func (s *Server) requireBalanceUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	_, hasAuthorization := r.Header[http.CanonicalHeaderKey("Authorization")]
	if hasAuthorization {
		rawKey := bearerToken(r.Header.Get("Authorization"))
		if rawKey == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
			return "", false
		}
		userID, err := s.apiKeyService.AuthenticateBalanceKey(r.Context(), rawKey)
		if errors.Is(err, apikeys.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "unauthenticated", nil)
			return "", false
		}
		if err != nil {
			s.log.Error("balance api key authentication failed", "error", err)
			writeError(w, http.StatusInternalServerError, "balance_auth_failed", nil)
			return "", false
		}
		return userID, true
	}

	user, ok := s.requireUser(w, r)
	if !ok {
		return "", false
	}
	return user.ID, true
}
```

- [ ] **Step 6: Format and run the HTTP tests**

Run:

```bash
cd services/api
gofmt -w internal/platform/http/server.go internal/platform/http/server_test.go
go test ./internal/platform/http -run 'TestBalance(Accepts|Rejects)' -count=1
```

Expected: all focused balance authentication tests pass.

- [ ] **Step 7: Document API-key balance requests**

Add this text immediately after the public/authenticated endpoint list in `README.md`:

````markdown
`GET /v1/balance` accepts either the authenticated `slai_session` cookie or the
user's SLAI API key. API-key clients can read their balance with:

```sh
curl -sS "$SLAI_API_URL/v1/balance" \
  -H "Authorization: Bearer $SLAI_RAW_API_KEY"
```

The `availableUnits`, `lifetimePurchasedUnits`, and `lifetimeUsedUnits` values
are integer micro-units. Divide them by `1,000,000` to display credits.
Suspended API keys retain read-only balance access; revoked keys do not.
````

- [ ] **Step 8: Run complete backend verification**

Run:

```bash
cd services/api
go test ./...
go vet ./...
```

Then, from the repository root, run:

```bash
git diff --check
git status --short
```

Expected: every Go package test and vet check passes, `git diff --check` prints nothing, and status lists only the intended HTTP, test, and README changes.

- [ ] **Step 9: Commit the endpoint, tests, and documentation**

```bash
git add README.md services/api/internal/platform/http/server.go services/api/internal/platform/http/server_test.go
git commit -m "feat: allow API key balance access"
```

---

## Final Verification

After both task commits, run from `services/api`:

```bash
go test ./internal/apikeys ./internal/platform/http -count=1
go test ./...
go vet ./...
```

From the repository root, confirm:

```bash
git log -3 --oneline
git status --short
```

Expected: all commands succeed; the two feature commits follow the approved design-document commit; the working tree is clean; and no migration, dependency, or response-schema change is present.

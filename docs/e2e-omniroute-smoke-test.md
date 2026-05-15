# SLAI + OmniRoute E2E Smoke Test

This runbook verifies the full prepaid AI credits loop against the patched
OmniRoute fork:

1. Admin creates or reuses a public credit package.
2. User signs up or logs in.
3. Admin manually tops up the user.
4. User creates or rotates an API key.
5. SLAI creates the linked OmniRoute key.
6. User calls OmniRoute with the SLAI-created raw key.
7. Admin triggers SLAI usage sync.
8. Usage appears in user and admin views.
9. Credits are deducted through the ledger.
10. Optional: balance is driven to zero and the key is suspended.

The automated path is `scripts/smoke-slai-omniroute.sh`. The UI paths are also
listed below so an operator can inspect each step in the current product.

## Prerequisites

Install or start:

- Patched OmniRoute fork: `https://github.com/seakleangnhak/OmniRoute`
- SLAI API, web app, and PostgreSQL from this repository
- `curl`
- `jq`
- A configured OmniRoute model/provider that can answer `/v1/chat/completions`

Use one shared management token for OmniRoute and SLAI. Do not commit it, paste
it in issue comments, or print it in logs.

## Required Environment

OmniRoute must run with API keys and management auth enabled:

```sh
export REQUIRE_API_KEY=true
export ALLOW_API_KEY_REVEAL=false
export OMNIROUTE_MANAGEMENT_TOKEN=<same-secret-as-slai>
```

SLAI must use the real OmniRoute HTTP client for this smoke test:

```sh
export OMNIROUTE_ENABLED=true
export OMNIROUTE_BASE_URL=http://localhost:<omniroute-port>
export OMNIROUTE_MANAGEMENT_TOKEN=<same-secret-as-omniroute>
export OMNIROUTE_USAGE_SYNC_MODE=call_logs
export OMNIROUTE_HTTP_TIMEOUT_SECONDS=15
export OMNIROUTE_CALL_LOG_LIMIT=100
```

Manual sync is enough for the script. Enable the worker when you also want to
verify scheduled sync:

```sh
export USAGE_SYNC_WORKER_ENABLED=true
export USAGE_SYNC_INTERVAL_SECONDS=60
export USAGE_SYNC_LOCK_KEY=slai_usage_sync
export USAGE_SYNC_BATCH_LIMIT=100
export USAGE_SYNC_START_DELAY_SECONDS=10
```

For local `npm run api:dev`, put these values in `services/api/.env`. The root
`api:dev` script loads that file before starting the Go API.

## Start Services Locally

Start PostgreSQL if it is not already running:

```sh
docker compose -f deploy/docker-compose.yml up -d postgres
```

Run migrations and seed the admin:

```sh
npm run api:migrate

cd services/api
go run ./cmd/slai-api seed-admin
cd ../..
```

Start SLAI API and web in separate terminals:

```sh
npm run api:dev
npm run web:dev
```

Verify API health:

```sh
curl -sS http://localhost:8080/healthz
curl -sS http://localhost:8080/readyz | jq .
```

## Run The Automated Smoke Test

Set test identities and URLs. If `services/api/.env` has admin seed credentials,
the script can load them automatically; explicit values still win.

```sh
export SLAI_API_URL=http://localhost:8080
export OMNIROUTE_BASE_URL=http://localhost:<omniroute-port>
export SLAI_ADMIN_EMAIL=admin@example.com
export SLAI_ADMIN_PASSWORD=change-me-admin-password
export SLAI_USER_EMAIL=smoke-developer@example.com
export SLAI_USER_PASSWORD=change-me-user-password
export OMNIROUTE_SMOKE_MODEL=gpt-4o-mini
```

Run:

```sh
scripts/smoke-slai-omniroute.sh
```

The script prints a pass/warn summary. It redacts passwords, management tokens,
and API keys from normal output. It prints the newly generated raw API key once
because the local smoke test must use it to call OmniRoute.

Useful options:

```sh
# Skip automatic loading of services/api/.env
export SLAI_SMOKE_LOAD_API_ENV=false

# Reuse/create a custom package name and displayed credit amount
export SLAI_SMOKE_PACKAGE_NAME="Smoke Test Pack"
export SLAI_SMOKE_PACKAGE_CREDITS=1000

# Stable run ID for idempotency keys; omit for timestamp-based IDs
export SLAI_SMOKE_RUN_ID=local-001

# Top-up amount used by the script
export SLAI_SMOKE_TOPUP_CREDITS=1000
export SLAI_SMOKE_TOPUP_MINOR=1000

# Also test key suspension after balance exhaustion
export SLAI_SMOKE_EXHAUST=true
```

The script is repeatable enough for local runs:

- It signs up the user, or logs in if the user already exists.
- It finds an existing smoke package by name before creating a new one.
- It activates the smoke package if it exists but is inactive.
- It rotates an existing user API key to obtain a one-time raw key.
- It uses idempotency keys for manual top-up and optional adjustment.

## What The Script Verifies

The automated smoke test checks:

- SLAI health and readiness.
- Admin login.
- Sync status reports `omniroute_enabled=true` and `sync_mode=call_logs`.
- Test user session is available.
- Admin package exists and is visible through `GET /v1/packages`.
- Admin manual top-up creates payment, ledger, and balance data.
- User API key create or rotate returns a one-time raw key.
- User and admin API key metadata endpoints do not return the raw key later.
- OmniRoute accepts a chat completion request with the SLAI-created raw key.
- `POST /v1/admin/usage/sync` succeeds.
- User usage, admin usage, user balance, user ledger, and user payments show the
  expected records after sync.
- SLAI bills synced call logs from token-based pricing rules. If OmniRoute
  exposes uncompressed token fields, those are used for SLAI billing and stored
  usage totals; compressed token counts remain OmniRoute-side only. OmniRoute
  `costUnits`, `cost_units`, or USD fields such as `costUsd` are preserved in
  raw usage JSON but no longer drive SLAI call-log billing.
- Re-running sync does not change balance unless another worker/log processed new
  events concurrently.
- Optional exhaustion path suspends the key when balance reaches zero or below.

## Current UI Inspection Paths

Use the web app at `http://localhost:3000` to inspect the same flow manually.

Public pages:

- Landing page: `/`
- Public packages/pricing: `/packages`

User portal:

- Sign up: `/signup`
- Sign in: `/login`
- Overview: `/dashboard`
- API key: `/dashboard/api-key`
- Usage: `/dashboard/usage`
- Billing, packages, top-ups, ledger: `/dashboard/billing`
- Settings/session: `/dashboard/settings`

Admin console:

- Admin sign in: `/admin/login`
- Admin overview: `/admin`
- Packages: `/admin/packages`
- Users and manual top-up: `/admin/users` and `/admin/users/{id}`
- Usage events: `/admin/usage`
- Sync status and manual sync: `/admin/sync`
- Audit logs: `/admin/audit`

## Manual API Flow

The script is the source of truth for curl examples, but these are the key API
surfaces it uses:

```http
POST /v1/auth/login
POST /v1/auth/signup
GET  /v1/me

GET  /v1/packages
GET  /v1/admin/packages
POST /v1/admin/packages
PATCH /v1/admin/packages/{id}

POST /v1/admin/payments/manual-topup
GET  /v1/payments
GET  /v1/balance
GET  /v1/ledger

GET    /v1/api-key
POST   /v1/api-key
POST   /v1/api-key/rotate
DELETE /v1/api-key

POST /v1/admin/usage/sync
GET  /v1/admin/usage/sync-status
GET  /v1/usage
GET  /v1/admin/usage
```

The OmniRoute model call is made directly to OmniRoute:

```sh
curl -sS -X POST "$OMNIROUTE_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SLAI_RAW_API_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"model\": \"$OMNIROUTE_SMOKE_MODEL\",
    \"messages\": [{\"role\": \"user\", \"content\": \"Reply with the word SLAI.\"}]
  }" | jq .
```

Do not store the raw key permanently. It should appear only in the create or
rotate response.

## Sync Status Debugging

The existing admin sync status endpoint is enough for E2E debugging and does not
expose secrets:

```sh
curl -sS "$SLAI_API_URL/v1/admin/usage/sync-status" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | jq .
```

Expected fields include:

- `worker_enabled`
- `currently_running`
- `last_started_at`
- `last_finished_at`
- `last_success_at`
- `last_error`
- `last_result`
- `next_run_at`
- `omniroute_enabled`
- `sync_mode`
- `worker_interval_seconds`
- `batch_limit`

No separate integration debug endpoint is required right now.

## Troubleshooting

If the script fails with `omniroute_enabled=false` or manual sync returns 501:

- Confirm `OMNIROUTE_ENABLED=true` in the **running** SLAI API process.
- Confirm `npm run api:dev` was restarted after editing `services/api/.env`.
- Confirm `OMNIROUTE_USAGE_SYNC_MODE=call_logs`.

If API key creation fails:

- Confirm `OMNIROUTE_BASE_URL` points to the patched OmniRoute server.
- Confirm the management token matches in both services.
- Confirm OmniRoute accepts `Authorization: Bearer <token>` on `/api/keys`.

If sync returns zero fetched events:

- Confirm the OmniRoute chat call completed successfully.
- Confirm the patched OmniRoute server wrote a call log.
- Confirm SLAI selected `/api/usage/call-logs`.
- Try increasing `OMNIROUTE_CALL_LOG_LIMIT`.
- Trigger manual sync again from `/admin/sync`.

If a usage event is ignored:

- Confirm the OmniRoute call log contains `apiKeyId`.
- Confirm `apiKeyId` matches `api_keys.omniroute_key_id` in SLAI.
- Confirm the API key was created through SLAI, not manually in OmniRoute.

If duplicate usage deducts twice:

- Inspect `usage_events` for uniqueness on `external_source` plus
  `external_event_id`.
- Inspect ledger idempotency keys. Usage debits should use
  `usage:{source}:{event_id}`.

If the automatic worker does not run:

- Confirm `USAGE_SYNC_WORKER_ENABLED=true`.
- Confirm the API process was restarted after changing env.
- Check `/admin/sync` or `GET /v1/admin/usage/sync-status`.
- Check API logs for lock-held or sync-failed messages.

## Cleanup

The script stores cookies and temporary responses in a temporary directory and
removes them on exit.

After manual testing, revoke the smoke-test key from the user portal
`/dashboard/api-key`, or with curl:

```sh
curl -sS -X DELETE "$SLAI_API_URL/v1/api-key" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

You can leave the smoke package active for future runs or manage it from
`/admin/packages`.

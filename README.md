# SLAI

SLAI is a prepaid AI API credits platform for developers.

SLAI owns the business layer:

- Users
- Sessions
- Credit packages
- Prepaid balances
- Manual top-ups
- API key ownership
- Usage billing
- Admin operations
- Audit logs

OmniRoute remains the AI gateway and provider routing layer.
Users call OmniRoute `/v1/*` directly with an OmniRoute-generated API key.
SLAI creates and manages that key, syncs OmniRoute usage logs, and deducts
prepaid credits through the SLAI ledger.

Credits never expire.
Credits and money use integer units only.
Every balance mutation goes through the credit ledger.

## Current Scope

Implemented now:

- Go API foundation
- PostgreSQL connection and readiness checks
- SQL migration runner
- Structured logging
- Email/password signup and login
- Argon2id password hashing
- HttpOnly session cookies
- USER and ADMIN roles
- ACTIVE and SUSPENDED users
- Admin seed command
- Public active package listing
- Admin package CRUD
- Credit balances
- Credit ledger
- Manual admin top-ups
- Bakong KHQR package checkout through the slai-payment service with signed callbacks
- Admin credit adjustments
- Admin audit log writing and listing
- API key creation, rotation, revocation, suspension, and resume
- One ACTIVE API key per user for MVP
- API key HMAC hash storage with display prefix only
- OmniRoute management-token HTTP client
- OmniRoute local/dev mode when integration is disabled
- Usage ingestion and idempotent credit deduction
- Mock usage ingestion for local testing
- Manual admin usage sync endpoint
- Automatic scheduled usage sync worker
- PostgreSQL advisory lock for usage sync
- Admin usage sync status endpoint
- Admin dashboard metrics endpoint
- Admin user management read and status endpoints
- MVP user dashboard and admin console
- Admin audit log UI page
- Docker Compose for local development

Not included yet:

- Official Bakong API settlement validation
- Stripe or other card payment provider integration
- Deep production reporting and support workflows
- Multi-key user management beyond the MVP one-active-key rule

## Repository Layout

```text
SLAI/
  apps/
    web/
  services/
    api/
  db/
    migrations/
  deploy/
  docs/
  scripts/
```

## Stack

- Monorepo
- Frontend: Next.js, TypeScript, Tailwind CSS
- Backend API: Go
- Database: PostgreSQL
- Local deployment: Docker Compose

## Docker Setup

Run the local stack:

```sh
docker compose -f deploy/docker-compose.yml up --build
```

Then open:

- Web: `http://localhost:3000`
- API health: `http://localhost:8080/healthz`
- API readiness: `http://localhost:8080/readyz`

The Compose stack starts Postgres, runs migrations, starts the API, and starts
the web app.

Docker defaults to local mode:

```sh
OMNIROUTE_ENABLED=false
USAGE_SYNC_WORKER_ENABLED=false
```

Local mode lets SLAI generate development API keys without calling OmniRoute.

## Local Development Without Docker

Install dependencies from the repository root:

```sh
npm install
```

Start PostgreSQL with the credentials from `services/api/.env.example`, then run:

```sh
npm run api:migrate
npm run api:dev
npm run web:dev
```

The migration command runs all unapplied SQL files in `db/migrations` and records
them in `schema_migrations`.

Seed an admin after migrations:

```sh
cd services/api
ADMIN_SEED_EMAIL=admin@example.com \
ADMIN_SEED_PASSWORD=change-me-admin-password \
go run ./cmd/slai-api seed-admin
```


## Docker Startup Migrations

The production API image runs `/app/docker-entrypoint.sh` before `serve`. On container startup it will:

1. run `/app/slai-api migrate up` when `SLAI_AUTO_MIGRATE` is unset or `true`, retrying while the database starts;
2. run `/app/slai-api seed-admin` when `SLAI_AUTO_SEED_ADMIN` is unset or `true` and both `ADMIN_SEED_EMAIL` and `ADMIN_SEED_PASSWORD` are set;
3. start the API server.

Migrations are idempotent because applied SQL files are tracked in `schema_migrations`, so every deploy can safely run the startup migration step. The admin seed command only creates the admin if it does not already exist; it does not reset an existing admin password.

Disable either startup step only for special operational flows:

```sh
SLAI_AUTO_MIGRATE=false
SLAI_AUTO_SEED_ADMIN=false
```

## API Environment

API variables live in `services/api/.env.example`.

Core settings:

- `APP_ENV`
- `HTTP_ADDR`
- `DATABASE_URL`
- `MIGRATIONS_DIR`
- `LOG_LEVEL`
- `SESSION_SECRET`
- `COOKIE_SECURE`
- `SESSION_TTL`
- `ADMIN_SEED_EMAIL`
- `ADMIN_SEED_PASSWORD`
- `SLAI_AUTO_MIGRATE`
- `SLAI_AUTO_SEED_ADMIN`
- `SLAI_STARTUP_DB_RETRIES`
- `SLAI_STARTUP_DB_RETRY_DELAY_SECONDS`
- `READINESS_TIMEOUT`
- `SHUTDOWN_TIMEOUT`

API key settings:

- `API_KEY_PEPPER`
- `API_KEY_PREFIX`

OmniRoute settings:

- `OMNIROUTE_ENABLED`
- `OMNIROUTE_BASE_URL`
- `OMNIROUTE_MANAGEMENT_TOKEN`
- `OMNIROUTE_USAGE_SYNC_MODE`
- `OMNIROUTE_HTTP_TIMEOUT_SECONDS`
- `OMNIROUTE_CALL_LOG_LIMIT`

Usage sync worker settings:

- `USAGE_SYNC_WORKER_ENABLED`
- `USAGE_SYNC_INTERVAL_SECONDS`
- `USAGE_SYNC_LOCK_KEY`
- `USAGE_SYNC_BATCH_LIMIT`
- `USAGE_SYNC_START_DELAY_SECONDS`

Storage and payment settings:

- `STORAGE_DIR` stores legacy/fallback KHQR assets and payment proof files.
- `PAYMENT_PROOF_MAX_MB` limits legacy proof uploads.
- `PAYMENT_QR_MAX_MB` limits fallback KHQR image uploads.

Automatic Bakong KHQR checkout settings:

- `SLAI_PAYMENT_ENABLED` turns on slai-payment checkout for package purchases.
- `SLAI_PAYMENT_BASE_URL` points SLAI at the slai-payment HTTP service.
- `SLAI_PAYMENT_API_KEY` is optional if slai-payment protects API calls.
- `SLAI_PAYMENT_CALLBACK_BASE_URL` is the public SLAI API base used in callback URLs.
- `SLAI_PAYMENT_CALLBACK_SECRET` verifies signed callbacks. Do not reuse public secrets.
- `SLAI_PAYMENT_MERCHANT_PREFIX` is passed to slai-payment when creating KHQR payments.
- `SLAI_PAYMENT_DEFAULT_EXPIRY` controls checkout expiry, for example `30m`.
- `SLAI_PAYMENT_HTTP_TIMEOUT` controls outbound provider calls.

Frontend settings live in `apps/web/.env.example`:

- `NEXT_PUBLIC_API_BASE_URL`

## Recommended Local Values

For local development without OmniRoute:

```sh
OMNIROUTE_ENABLED=false
USAGE_SYNC_WORKER_ENABLED=false
```

For a production-like OmniRoute test:

```sh
OMNIROUTE_ENABLED=true
OMNIROUTE_BASE_URL=http://localhost:4000
OMNIROUTE_MANAGEMENT_TOKEN=<same-secret-as-omniroute>
OMNIROUTE_USAGE_SYNC_MODE=call_logs
OMNIROUTE_HTTP_TIMEOUT_SECONDS=15
OMNIROUTE_CALL_LOG_LIMIT=100
USAGE_SYNC_WORKER_ENABLED=true
USAGE_SYNC_INTERVAL_SECONDS=60
USAGE_SYNC_START_DELAY_SECONDS=10
```

For local Bakong KHQR checkout with `slai-payment` running on port 8090:

```sh
SLAI_PAYMENT_ENABLED=true
SLAI_PAYMENT_BASE_URL=http://localhost:8090
SLAI_PAYMENT_CALLBACK_BASE_URL=http://localhost:8080
SLAI_PAYMENT_CALLBACK_SECRET=<same-secret-as-slai-payment>
SLAI_PAYMENT_MERCHANT_PREFIX=SLAI
SLAI_PAYMENT_DEFAULT_EXPIRY=30m
```

`SLAI_PAYMENT_CALLBACK_BASE_URL` must be reachable by the slai-payment service. If slai-payment runs outside the same host network, use a public tunnel or deploy both services on a network where callbacks can reach SLAI.

## Bakong KHQR Auto Payment MVP

SLAI supports Bakong KHQR package checkout through the local `slai-payment` service:

1. Admin enables and monitors provider configuration at `/admin/settings/payments`.
2. User chooses a package from `/packages` or `/dashboard/billing`.
3. SLAI creates a local `pending_payment` row and asks `slai-payment` to create a KHQR payment.
4. The checkout page shows the provider-generated KHQR image, amount, reference, and expiry.
5. User pays in their Bakong or bank app.
6. `slai-payment` matches the bank/Telegram confirmation and sends SLAI a signed `payment.paid` callback.
7. SLAI verifies the HMAC signature over `timestamp + "." + raw_json_body`, validates amount/currency/reference, marks the payment `paid`, and creates a ledger `payment_credit`.
8. Ledger crediting uses idempotency key `slai_payment_paid:{payment_id}`, so repeated callbacks do not double-credit.
9. If `slai-payment` expires a pending checkout, it sends a signed `payment.expired` callback and SLAI marks the local payment `expired` without crediting the ledger.

Callback headers are:

- `X-SLAI-Payment-Timestamp`
- `X-SLAI-Payment-Signature: v1=<hex-hmac-sha256>`
- `X-SLAI-Payment-ID`

Provider statuses map to SLAI statuses: `PENDING` -> `pending_payment`, `PAID` -> `paid`, `EXPIRED` -> `expired`, and unsafe or unknown provider states -> `needs_review`. The `SLAI_PAYMENT_EXPIRY_CHECK_INTERVAL` worker setting belongs to the `slai-payment` service; SLAI only receives and verifies its signed expiry callback. Legacy manual proof endpoints remain for old records, but new checkouts should use slai-payment when `SLAI_PAYMENT_ENABLED=true`.

## OmniRoute Requirement

Use the patched OmniRoute fork or an upstream build with equivalent management
auth support:

```text
https://github.com/seakleangnhak/OmniRoute
```

OmniRoute must accept trusted server-to-server management requests with:

```http
Authorization: Bearer <OMNIROUTE_MANAGEMENT_TOKEN>
```

Recommended OmniRoute environment:

```sh
REQUIRE_API_KEY=true
ALLOW_API_KEY_REVEAL=false
OMNIROUTE_MANAGEMENT_TOKEN=<long-random-secret>
```

Use the same management token in SLAI:

```sh
OMNIROUTE_MANAGEMENT_TOKEN=<same-secret>
```

Do not commit the token.
Do not log the token.

## API Key Lifecycle

For MVP, each user can have one ACTIVE API key.

SLAI stores only:

- API key HMAC hash
- Display prefix
- Status
- Local metadata
- OmniRoute key ID when OmniRoute is enabled

SLAI never stores the raw API key.
The raw key is returned only once when it is created or rotated.

Create flow when `OMNIROUTE_ENABLED=true`:

1. The user calls `POST /v1/api-key`.
2. SLAI calls OmniRoute `POST /api/keys` with management-token auth.
3. OmniRoute returns the raw API key once.
4. SLAI hashes the raw key and stores only hash plus prefix.
5. SLAI stores the OmniRoute key ID in `api_keys.omniroute_key_id`.
6. SLAI returns the raw key once to the user.

Create flow when `OMNIROUTE_ENABLED=false`:

1. The user calls `POST /v1/api-key`.
2. SLAI generates a local development API key.
3. SLAI hashes the raw key and stores only hash plus prefix.
4. SLAI returns the raw key once to the user.
5. No OmniRoute management request is made.

Rotate flow:

1. SLAI revokes the previous key locally.
2. SLAI disables or deletes the old OmniRoute key when enabled.
3. SLAI creates a new key.
4. SLAI returns the new raw key once.

Revoke flow:

1. SLAI marks the key `REVOKED`.
2. SLAI sets `revoked_at`.
3. SLAI disables or deletes the OmniRoute key when enabled.

Suspend and resume flow:

1. Admins can suspend, resume, or revoke a user key.
2. SLAI updates local key status.
3. SLAI calls OmniRoute `PATCH /api/keys/{id}` when enabled.
4. Resume is allowed only when the user has a positive balance.

## Usage Billing Flow

Users call OmniRoute directly:

```text
User application -> OmniRoute /v1/* -> provider
```

SLAI bills asynchronously:

```text
SLAI usage sync -> OmniRoute call logs -> usage_events -> credit ledger -> balance
```

Each usage event is identified by:

- `external_source`
- `external_event_id`

The database unique constraint on those fields prevents double ingestion.
The ledger mutation also uses an idempotency key:

```text
usage:{external_source}:{external_event_id}
```

If a duplicate event arrives, SLAI returns a duplicate result and does not deduct
credits again.

If an event references an unknown API key, SLAI ignores it and does not deduct
credits.

If async billing drives a balance below zero, SLAI keeps the negative balance and
suspends the API key to prevent future usage.

## Pricing

Pricing rules are stored in `pricing_rules`.

The initial migration seeds a default active rule:

```text
provider = NULL
model = NULL
input_cost_units_per_1k = 1
output_cost_units_per_1k = 1
active = true
```

Pricing lookup order:

1. Provider plus model exact match
2. Model-only match
3. Provider-only match
4. Default rule where provider and model are both NULL

Cost uses integer math only:

```text
input_cost = ceil(input_tokens / 1000) * input_cost_units_per_1k
output_cost = ceil(output_tokens / 1000) * output_cost_units_per_1k
total_cost = input_cost + output_cost
```

For OmniRoute call-log sync, SLAI prefers uncompressed token fields when
OmniRoute exposes them, then falls back to the standard token fields. OmniRoute
cost fields such as `costUsd` are kept in raw usage JSON for audit context, but
SLAI credit debits are calculated from SLAI pricing rules and the billable token
counts stored on the usage event.

Zero tokens cost zero.
There is no minimum charge yet.

## Automatic Usage Sync Worker

The automatic scheduled usage sync worker is implemented.

When enabled, the API process starts a background worker on startup.
The worker waits for `USAGE_SYNC_START_DELAY_SECONDS`, then runs every
`USAGE_SYNC_INTERVAL_SECONDS`.

Each tick:

1. Acquires a PostgreSQL advisory lock.
2. Skips the tick if another process holds the lock.
3. Calls the existing usage sync service.
4. Processes OmniRoute call logs.
5. Deducts credits through the ledger.
6. Suspends keys when balances are zero or negative.
7. Releases the advisory lock.

The advisory lock protects multi-replica deployments.
You can still choose to run only one worker-enabled API replica in production,
but multiple replicas are protected by the database lock.

Recommended production settings:

```sh
USAGE_SYNC_WORKER_ENABLED=true
USAGE_SYNC_INTERVAL_SECONDS=60
USAGE_SYNC_START_DELAY_SECONDS=10
USAGE_SYNC_LOCK_KEY=slai_usage_sync
USAGE_SYNC_BATCH_LIMIT=100
```

## Sync Status

Admins can inspect in-memory sync status:

```http
GET /v1/admin/usage/sync-status
```

The response includes:

- `worker_enabled`
- `last_started_at`
- `last_finished_at`
- `last_success_at`
- `last_error`
- `last_result`
- `next_run_at`
- `currently_running`

Manual sync updates the same status tracker.

## Admin Audit Logs

Admin actions write audit log rows for package changes, manual top-ups, credit
adjustments, user status changes, and API key actions.

Admins can list audit logs with filters:

```http
GET /v1/admin/audit-logs?action=manual_topup_created&target_type=payment
```

Supported query parameters:

- `admin_id`
- `action`
- `target_type`
- `target_id`
- `from`
- `to`
- `limit`
- `offset`

The response joins the admin user email for visibility and does not expose
password hashes, session tokens, API key hashes, raw API keys, peppers,
management tokens, or secret values. The Next.js admin console includes an
Audit Logs page at `/admin/audit`.

## Main API Endpoints

Public and authenticated endpoints:

- `GET /healthz`
- `GET /readyz`
- `POST /v1/auth/signup`
- `POST /v1/auth/login`
- `POST /v1/auth/logout`
- `GET /v1/me`
- `GET /v1/packages`
- `GET /v1/balance`
- `GET /v1/ledger`
- `GET /v1/usage`
- `GET /v1/api-key`
- `POST /v1/api-key`
- `POST /v1/api-key/rotate`
- `DELETE /v1/api-key`

Admin endpoints:

- `GET /v1/admin/dashboard`
- `GET /v1/admin/packages`
- `POST /v1/admin/packages`
- `PATCH /v1/admin/packages/{id}`
- `POST /v1/admin/payments/manual-topup`
- `POST /v1/admin/ledger/adjustments`
- `POST /v1/admin/usage/sync`
- `GET /v1/admin/usage/sync-status`
- `GET /v1/admin/usage`
- `GET /v1/admin/audit-logs`
- `GET /v1/admin/users`
- `GET /v1/admin/users/{id}`
- `PATCH /v1/admin/users/{id}/status`
- `GET /v1/admin/users/{id}/api-key`
- `POST /v1/admin/users/{id}/api-key/suspend`
- `POST /v1/admin/users/{id}/api-key/resume`
- `POST /v1/admin/users/{id}/api-key/revoke`

Local and internal testing endpoint:

- `POST /v1/internal/usage/mock-event`

The mock usage endpoint currently requires an ADMIN session.

## Manual Top-Up Flow

Manual top-ups are admin-created only.

A manual top-up creates all of the following in one database transaction:

- A `payments` row
- A `credit_ledger_entries` row with type `payment_credit`
- A `credit_balances` update
- An `admin_audit_logs` row

Use an idempotency key for safe retries.

Example:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/payments/manual-topup" \
  -b admin.cookies \
  -c admin.cookies \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: topup-example-001' \
  -d '{
    "userId": "<user-id>",
    "packageId": null,
    "amountMinor": 1000,
    "currency": "USD",
    "creditUnits": 1000,
    "note": "Smoke test top-up"
  }'
```

## Admin Adjustment Flow

Admin adjustments require a reason.

An adjustment creates all of the following in one database transaction:

- A `credit_ledger_entries` row
- A `credit_balances` update
- An `admin_audit_logs` row

Positive adjustments use type `admin_adjustment_credit`.
Negative adjustments use type `admin_adjustment_debit`.

## Mock Usage Ingestion

For local testing without OmniRoute, create a user key and ingest a mock event:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/internal/usage/mock-event" \
  -b admin.cookies \
  -c admin.cookies \
  -H 'Content-Type: application/json' \
  -d '{
    "api_key_id": "<slai-api-key-id>",
    "external_event_id": "mock-001",
    "model": "gpt-5.5",
    "provider": "openai",
    "input_tokens": 7240,
    "output_tokens": 357,
    "occurred_at": "2026-04-28T10:00:00Z"
  }'
```

Reusing the same `external_event_id` returns a duplicate result and does not
charge again.

## E2E Smoke Test

The full SLAI plus OmniRoute smoke test guide is here:

```text
docs/e2e-omniroute-smoke-test.md
```

An optional helper script is here:

```text
scripts/smoke-slai-omniroute.sh
```

Required script environment:

- `SLAI_API_URL`
- `SLAI_ADMIN_EMAIL`
- `SLAI_ADMIN_PASSWORD`
- `SLAI_USER_EMAIL`
- `SLAI_USER_PASSWORD`
- `OMNIROUTE_BASE_URL`

Optional script environment:

- `OMNIROUTE_MANAGEMENT_TOKEN`
- `OMNIROUTE_SMOKE_MODEL`
- `SLAI_SMOKE_TOPUP_UNITS`
- `SLAI_SMOKE_TOPUP_MINOR`

## Verification Commands

From the repository root:

```sh
find services/api -name '*.go' -print0 | xargs -0 gofmt -w
```

From `services/api`:

```sh
go test ./...
go vet ./...
```

From the repository root:

```sh
npm --workspace apps/web run build
docker compose -f deploy/docker-compose.yml config
bash -n scripts/smoke-slai-omniroute.sh
```

## Security Notes

- Raw API keys are never stored.
- Raw API keys are returned only once on create or rotate.
- API key hashes use an application pepper.
- The management token is required only for SLAI-to-OmniRoute management calls.
- Do not print the management token in logs.
- Do not include raw API keys in structured logs.
- Session cookies are HttpOnly.
- Set `COOKIE_SECURE=true` when serving over HTTPS.

## Deployment Notes

The Docker Compose file is intended for local development and as a starting point
for Dokploy or Traefik deployment.

For production-like deployment:

- Use managed PostgreSQL or a durable Postgres volume.
- Set strong values for `SESSION_SECRET` and `API_KEY_PEPPER`.
- Set `COOKIE_SECURE=true` behind HTTPS.
- Set `OMNIROUTE_ENABLED=true`.
- Set a long random `OMNIROUTE_MANAGEMENT_TOKEN`.
- Enable the usage sync worker on one or more API replicas.
- Let the PostgreSQL advisory lock protect against concurrent sync runs.

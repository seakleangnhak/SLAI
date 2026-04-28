# SLAI

SLAI is a prepaid AI API credits platform for developers. SLAI owns users,
prepaid credits, balances, manual top-ups, API key ownership, usage billing,
admin operations, and audit logs. OmniRoute remains the AI gateway and provider
routing layer.

Users call OmniRoute `/v1/*` directly with an OmniRoute-generated API key that
SLAI creates and manages. SLAI syncs OmniRoute usage logs and deducts prepaid
credits. Credits never expire.

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
the web app. Local Docker defaults to `OMNIROUTE_ENABLED=false`.

## Local Development Without Docker

Start PostgreSQL with the credentials from `services/api/.env.example`, then run:

```sh
npm install
npm run api:migrate
npm run api:dev
npm run web:dev
```

The migration command runs all unapplied `*.sql` files in `db/migrations` and
records them in `schema_migrations`.

Seed an admin after migrations:

```sh
cd services/api
ADMIN_SEED_EMAIL=admin@example.com \
ADMIN_SEED_PASSWORD=change-me-admin-password \
go run ./cmd/slai-api seed-admin
```

## Environment

API variables live in `services/api/.env.example`:

- `APP_ENV`
- `HTTP_ADDR`
- `DATABASE_URL`
- `MIGRATIONS_DIR`
- `LOG_LEVEL`
- `SESSION_SECRET`
- `COOKIE_SECURE`
- `SESSION_TTL`
- `API_KEY_PEPPER`
- `API_KEY_PREFIX`
- `ADMIN_SEED_EMAIL`
- `ADMIN_SEED_PASSWORD`
- `READINESS_TIMEOUT`
- `SHUTDOWN_TIMEOUT`
- `OMNIROUTE_ENABLED`
- `OMNIROUTE_BASE_URL`
- `OMNIROUTE_MANAGEMENT_TOKEN`
- `OMNIROUTE_USAGE_SYNC_MODE`
- `OMNIROUTE_HTTP_TIMEOUT_SECONDS`
- `OMNIROUTE_CALL_LOG_LIMIT`
- `USAGE_SYNC_WORKER_ENABLED`
- `USAGE_SYNC_INTERVAL_SECONDS`
- `USAGE_SYNC_LOCK_KEY`
- `USAGE_SYNC_BATCH_LIMIT`
- `USAGE_SYNC_START_DELAY_SECONDS`

Web variables live in `apps/web/.env.example`:

- `NEXT_PUBLIC_API_BASE_URL`

## Current Implemented Scope

Implemented now:

- Go API process with structured JSON logging
- `/healthz` endpoint returning `OK`
- `/readyz` endpoint checking PostgreSQL connectivity
- PostgreSQL pool setup
- Minimal SQL migration runner
- Initial schema for users, sessions, packages, payments, balances, ledger, API
  keys, usage events, pricing, audit logs, and OmniRoute sync state
- Email/password signup and login with Argon2id password hashes
- HttpOnly session cookie backed by the `sessions` table
- USER and ADMIN roles
- ACTIVE and SUSPENDED user statuses
- `GET /v1/me`, `GET /v1/packages`, `GET /v1/balance`, and `GET /v1/ledger`
- Admin package create/list/update endpoints
- Manual admin top-up with payment row, ledger row, balance update,
  idempotency key, and audit log
- Admin credit adjustment with required reason, ledger row, balance update,
  idempotency key, and audit log
- API key creation, rotation, revocation, suspension, and balance-gated resume
- One active API key per user for MVP
- API key storage with HMAC hash plus display prefix only
- Raw API keys returned once on create or rotate
- OmniRoute-backed API key management when enabled
- Local/dev API key generation when OmniRoute is disabled
- Usage ingestion service for OmniRoute call logs and local/mock events
- Pricing lookup with a default active pricing rule
- Integer-only ceil-per-1k token billing
- Idempotent usage billing through `usage_events` and `credit_ledger_entries`
- Automatic API key suspension when async usage billing drives balance to zero
  or below
- PostgreSQL guard preventing direct `credit_balances` mutation outside the
  ledger service transaction path
- Admin seed command: `slai-api seed-admin`
- Real OmniRoute HTTP client plus stub client for local mode and tests
- Automatic scheduled usage sync worker with PostgreSQL advisory locking
- Admin sync-status endpoint
- Next.js app shells for landing, login, user dashboard, and admin dashboard
- Docker Compose for Postgres, migrations, API, and web

Not implemented yet:

- Stripe or external payment-provider flows
- Deep production frontend UI beyond the current shells

## API Surface

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
- `GET /v1/admin/packages`
- `POST /v1/admin/packages`
- `PATCH /v1/admin/packages/{id}`
- `POST /v1/admin/payments/manual-topup`
- `POST /v1/admin/ledger/adjustments`
- `POST /v1/internal/usage/mock-event`
- `POST /v1/admin/usage/sync`
- `GET /v1/admin/usage/sync-status`
- `GET /v1/admin/usage`
- `GET /v1/admin/users/{id}/api-key`
- `POST /v1/admin/users/{id}/api-key/suspend`
- `POST /v1/admin/users/{id}/api-key/resume`
- `POST /v1/admin/users/{id}/api-key/revoke`

## API Key Lifecycle

For MVP, each user can have one `ACTIVE` API key. The database supports multiple
keys later, but the service logic and partial unique index enforce the MVP rule.

Create:

- SLAI creates an OmniRoute key when `OMNIROUTE_ENABLED=true`.
- SLAI creates a local dev key when `OMNIROUTE_ENABLED=false`.
- The raw key is returned once.
- SLAI stores only `key_hash` and `key_prefix`.

Rotate:

- SLAI revokes the current active or suspended key.
- SLAI deletes/disables the old OmniRoute key when enabled.
- SLAI creates a new key and returns the new raw key once.

Revoke:

- SLAI marks the local key `REVOKED` and sets `revoked_at`.
- SLAI deletes/disables the OmniRoute key when enabled.

Suspend and resume:

- Suspend marks the local key `SUSPENDED` and sends `isActive=false` to
  OmniRoute when enabled.
- Resume only succeeds when the user balance is greater than zero.
- Resume marks the key `ACTIVE` and sends `isActive=true` to OmniRoute when
  enabled.

## Usage Ingestion

SLAI bills usage asynchronously from OmniRoute call logs. Users still call
OmniRoute `/v1/*` directly with the OmniRoute-generated key. SLAI fetches
`GET /api/usage/call-logs`, maps OmniRoute `apiKeyId` back to
`api_keys.omniroute_key_id`, writes a `usage_events` row, and deducts credits
through the ledger in the same database transaction.

Usage idempotency is enforced by `usage_events.external_source` plus
`usage_events.external_event_id`. Ledger idempotency uses
`usage:{external_source}:{external_event_id}`. Replaying the same external event
returns a duplicate result and does not deduct credits again.

Pricing uses active `pricing_rules` in this order:

1. Provider and model match
2. Model-only match
3. Provider-only match
4. Default rule where provider and model are both `NULL`

The initial migration seeds a safe default rule of 1 input credit unit and 1
output credit unit per started 1,000 tokens. Calculation uses integer math only:

```text
ceil(tokens / 1000) * units_per_1k
```

Async usage can temporarily make a balance negative. When billing leaves the
balance at or below zero, SLAI marks the API key `SUSPENDED`; if OmniRoute is
enabled, SLAI also sends `isActive=false` through the OmniRoute client.

For local testing, admins can submit a mock event:

```sh
curl -X POST http://localhost:8080/v1/internal/usage/mock-event \
  -H 'Content-Type: application/json' \
  --cookie 'slai_session=...' \
  -d '{
    "api_key_id": "...",
    "external_event_id": "mock-001",
    "model": "gpt-5.5",
    "provider": "openai",
    "input_tokens": 7240,
    "output_tokens": 357,
    "occurred_at": "2026-04-28T10:00:00Z"
  }'
```

## Automatic Usage Sync Worker

The automatic scheduled usage sync worker is implemented.

When `USAGE_SYNC_WORKER_ENABLED=true`, the API starts a background worker. The
worker waits `USAGE_SYNC_START_DELAY_SECONDS`, then runs every
`USAGE_SYNC_INTERVAL_SECONDS`. Each tick uses the same idempotent billing service
as `POST /v1/admin/usage/sync`.

Recommended production values:

```sh
USAGE_SYNC_WORKER_ENABLED=true
USAGE_SYNC_INTERVAL_SECONDS=60
USAGE_SYNC_START_DELAY_SECONDS=10
USAGE_SYNC_LOCK_KEY=slai_usage_sync
USAGE_SYNC_BATCH_LIMIT=
```

If `USAGE_SYNC_BATCH_LIMIT` is empty or non-positive, SLAI uses
`OMNIROUTE_CALL_LOG_LIMIT`.

The worker uses a PostgreSQL advisory lock based on `USAGE_SYNC_LOCK_KEY`, so
multiple API replicas do not process the same sync batch concurrently. Running
only one worker-enabled replica is still the simplest production setup; the DB
lock protects against accidental overlap.

When `OMNIROUTE_ENABLED=false`, an enabled worker logs that sync is skipped and
startup remains healthy.

## Sync Status Endpoint

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
- `last_result.fetched`
- `last_result.billed`
- `last_result.duplicate`
- `last_result.ignored`
- `last_result.failed`
- `last_result.suspended_keys`
- `next_run_at`
- `currently_running`

Manual admin sync remains available:

```http
POST /v1/admin/usage/sync
```

Manual sync uses `OMNIROUTE_USAGE_SYNC_MODE`. `call_logs` mode uses
`GET /api/usage/call-logs?limit=...`; OmniRoute does not currently support a
`since` query for this endpoint, so SLAI relies on usage-event idempotency.
`usage_history` mode calls `GET /api/usage/history`, but SLAI treats it as
unsupported unless the response contains stable event IDs and `apiKeyId`.
`call_logs` is preferred.

## OmniRoute Setup

Use the patched OmniRoute fork or an upstream build with equivalent trusted
management-auth support:

- https://github.com/seakleangnhak/OmniRoute

Recommended OmniRoute environment:

```sh
REQUIRE_API_KEY=true
ALLOW_API_KEY_REVEAL=false
OMNIROUTE_MANAGEMENT_TOKEN=<long-random-secret>
```

SLAI environment for a real OmniRoute deployment:

```sh
OMNIROUTE_ENABLED=true
OMNIROUTE_BASE_URL=https://your-omniroute-domain.com
OMNIROUTE_MANAGEMENT_TOKEN=<same-secret>
OMNIROUTE_USAGE_SYNC_MODE=call_logs
OMNIROUTE_HTTP_TIMEOUT_SECONDS=15
OMNIROUTE_CALL_LOG_LIMIT=100
USAGE_SYNC_WORKER_ENABLED=true
USAGE_SYNC_INTERVAL_SECONDS=60
USAGE_SYNC_START_DELAY_SECONDS=10
USAGE_SYNC_LOCK_KEY=slai_usage_sync
USAGE_SYNC_BATCH_LIMIT=
```

With this configuration, users call OmniRoute `/v1/*` directly using keys
created through SLAI. SLAI creates, disables, enables, deletes, and lists keys
through OmniRoute `/api/keys*`, then syncs `/api/usage/call-logs` to deduct
prepaid credits.

## Local Mode

Local development defaults to `OMNIROUTE_ENABLED=false`. In this mode SLAI still
creates user API keys, but they are local `API_KEY_PREFIX` keys with no
`omniroute_key_id`. Usage billing can still be exercised with the admin mock
usage endpoint.

## End-to-End Smoke Test

Use the E2E guide to verify SLAI against the patched OmniRoute fork with
management-token auth:

- `docs/e2e-omniroute-smoke-test.md`

The guide covers admin login, user signup, manual top-up, API key creation, an
OmniRoute `/v1/chat/completions` call, manual usage sync, duplicate sync
behavior, balance and ledger checks, key suspension, and the sync-status
endpoint.

A helper script is available for the common curl flow:

```sh
SLAI_API_URL=http://localhost:8080 \
SLAI_ADMIN_EMAIL=admin@example.com \
SLAI_ADMIN_PASSWORD=change-me-admin-password \
SLAI_USER_EMAIL=smoke-user@example.com \
SLAI_USER_PASSWORD=change-me-user-password \
OMNIROUTE_BASE_URL=http://localhost:4000 \
scripts/smoke-slai-omniroute.sh
```

## Useful Commands

```sh
cd services/api
go test ./...
go vet ./...
```

```sh
npm --workspace apps/web run build
docker compose -f deploy/docker-compose.yml config
bash -n scripts/smoke-slai-omniroute.sh
```

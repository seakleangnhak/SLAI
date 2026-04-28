# SLAI

SLAI is a prepaid AI API credits platform for developers. SLAI owns users, balances, payments/top-ups, API key ownership, usage billing, admin operations, and audit logs. OmniRoute remains the AI gateway and provider routing layer.

## Stack

- Monorepo
- Frontend: Next.js, TypeScript, Tailwind CSS
- Backend API: Go
- Database: PostgreSQL
- Local deployment: Docker Compose

## Local Setup

Run everything through Docker Compose:

```sh
docker compose -f deploy/docker-compose.yml up --build
```

Then open:

- Web: http://localhost:3000
- API health: http://localhost:8080/healthz
- API readiness: http://localhost:8080/readyz

## Local Development Without Docker

Start PostgreSQL with the credentials in `services/api/.env.example`, then run:

```sh
npm install
npm run api:migrate
npm run api:dev
npm run web:dev
```

The API migration command currently runs all unapplied `*.sql` files in `db/migrations` and records them in `schema_migrations`.

Create an admin user after migrations with:

```sh
cd services/api
ADMIN_SEED_EMAIL=admin@example.com ADMIN_SEED_PASSWORD=change-me-admin-password go run ./cmd/slai-api seed-admin
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

## Current Scope

Implemented now:

- Go API process with structured JSON logging
- `/healthz` endpoint returning `OK`
- `/readyz` endpoint checking PostgreSQL connectivity
- PostgreSQL pool setup
- Minimal SQL migration runner
- Initial schema for users, sessions, packages, payments, balances, ledger, API keys, usage events, pricing, audit logs, and OmniRoute sync state
- Email/password signup and login with Argon2id password hashes
- HttpOnly session cookie backed by the `sessions` table
- `GET /v1/me`, `GET /v1/packages`, `GET /v1/balance`, and `GET /v1/ledger`
- Admin package create/list/update endpoints
- Manual admin top-up flow with payment row, ledger row, balance update, idempotency key, and audit log
- Admin credit adjustment flow with required reason, ledger row, balance update, idempotency key, and audit log
- API key creation, rotation, revocation, suspension, and balance-gated resume
- API key storage with hash plus display prefix only; raw keys are returned once on create/rotate
- OmniRoute-backed key creation hooks when enabled, with local/dev key generation when disabled
- Usage ingestion service for OmniRoute call logs and local/mock events
- Pricing lookup with a default active pricing rule and integer-only ceil-per-1k token billing
- Idempotent usage billing through `usage_events` plus `credit_ledger_entries`
- Automatic API key suspension when async usage billing drives balance to zero or below
- Database guard preventing direct `credit_balances` mutation outside the ledger service transaction path
- Admin seed command: `slai-api seed-admin`
- Real OmniRoute HTTP client plus stub client for local mode/tests
- Automatic scheduled usage sync worker with PostgreSQL advisory locking
- Next.js app shells for landing, login, user dashboard, and admin dashboard
- Docker Compose for Postgres, migrations, API, and web

Not implemented yet:

- Stripe or external payments

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

For MVP, each user can have one `ACTIVE` API key. The database supports more keys later, but the service and partial unique index enforce the MVP rule now.

- Create: returns the raw API key once and stores only `key_hash` plus `key_prefix`.
- Rotate: revokes the current active/suspended key, creates a new key, and returns the new raw key once.
- Revoke: marks the local key `REVOKED`, sets `revoked_at`, and deletes/disables the OmniRoute key when enabled.
- Suspend: marks the local key `SUSPENDED` and sends an inactive update to OmniRoute when enabled.
- Resume: only succeeds when the user balance is greater than zero, then marks the key `ACTIVE` and sends an active update to OmniRoute when enabled.

When `OMNIROUTE_ENABLED=false`, SLAI runs in local/dev mode and generates a local `API_KEY_PREFIX` key such as `sk_slai_...`. No OmniRoute key id is stored. When `OMNIROUTE_ENABLED=true`, SLAI calls OmniRoute `POST /api/keys` and uses the raw key returned by OmniRoute as the user-visible key. Suspend/resume uses `PATCH /api/keys/{id}` with `isActive=false/true`, and revoke/delete uses `DELETE /api/keys/{id}`.

## Usage Ingestion

SLAI bills usage asynchronously from OmniRoute call logs. Users still call OmniRoute `/v1/*` directly with the OmniRoute-generated key; SLAI fetches `GET /api/usage/call-logs`, maps OmniRoute `apiKeyId` back to local `api_keys.omniroute_key_id`, writes a `usage_events` row, and deducts credits through the ledger in the same database transaction.

Usage idempotency is enforced by `usage_events.external_source` plus `usage_events.external_event_id`, and ledger idempotency uses `usage:{external_source}:{external_event_id}`. A replay returns a duplicate result and does not deduct credits again.

Pricing uses active `pricing_rules` in this order: provider+model, model-only, provider-only, then the default rule where both are `NULL`. The initial migration seeds a safe default rule of 1 input credit unit and 1 output credit unit per started 1,000 tokens. Calculation uses integer math only: `ceil(tokens / 1000) * units_per_1k`.

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

Async usage can temporarily make a balance negative. When billing leaves the balance at or below zero, SLAI marks the API key `SUSPENDED`; if OmniRoute is enabled, SLAI also sends `isActive=false` through the OmniRoute client abstraction.

`POST /v1/admin/usage/sync` reads `OMNIROUTE_USAGE_SYNC_MODE` and calls the OmniRoute interface. `call_logs` mode uses `GET /api/usage/call-logs?limit=...`; the endpoint does not currently support a `since` query, so SLAI relies on usage-event idempotency. `usage_history` mode calls `GET /api/usage/history`, but SLAI treats it as unsupported unless the response contains stable event ids and `apiKeyId`; `call_logs` is preferred.

## Automatic Usage Sync Worker

The API can run a background worker that periodically syncs OmniRoute usage and bills credits through the same idempotent usage service used by `POST /v1/admin/usage/sync`. The worker is disabled by default for local development.

Recommended production values:

```sh
USAGE_SYNC_WORKER_ENABLED=true
USAGE_SYNC_INTERVAL_SECONDS=60
USAGE_SYNC_START_DELAY_SECONDS=10
USAGE_SYNC_LOCK_KEY=slai_usage_sync
USAGE_SYNC_BATCH_LIMIT=
```

If `USAGE_SYNC_BATCH_LIMIT` is empty or non-positive, SLAI uses `OMNIROUTE_CALL_LOG_LIMIT`. Each tick acquires a PostgreSQL advisory lock using `USAGE_SYNC_LOCK_KEY`, so multiple API replicas can be deployed without running billing sync concurrently. Running only one worker-enabled replica is still the simplest operational setup; the DB lock protects against accidental overlap.

When `OMNIROUTE_ENABLED=false`, an enabled worker logs that sync is skipped and leaves startup healthy. Manual admin sync remains available at `POST /v1/admin/usage/sync`, and `GET /v1/admin/usage/sync-status` returns in-memory visibility for the latest run, including timestamps, last error, next scheduled run, and result counters.

## OmniRoute Requirement

Use [seakleangnhak/OmniRoute](https://github.com/seakleangnhak/OmniRoute) or an upstream build with equivalent trusted management-auth support. SLAI sends `Authorization: Bearer <OMNIROUTE_MANAGEMENT_TOKEN>` to OmniRoute management APIs.

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

With this configuration, users call OmniRoute `/v1/*` directly using keys created through SLAI. SLAI creates, disables, enables, deletes, and lists keys through OmniRoute `/api/keys*`, then syncs `/api/usage/call-logs` to deduct prepaid credits. Local mode still works with `OMNIROUTE_ENABLED=false`.

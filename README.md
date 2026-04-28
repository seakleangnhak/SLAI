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
- OmniRoute client interface and stub client
- Next.js app shells for landing, login, user dashboard, and admin dashboard
- Docker Compose for Postgres, migrations, API, and web

Not implemented yet:

- Stripe or external payments
- Real OmniRoute management and usage HTTP client calls

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

When `OMNIROUTE_ENABLED=false`, SLAI runs in local/dev mode and generates a local `API_KEY_PREFIX` key such as `sk_slai_...`. No OmniRoute key id is stored. When `OMNIROUTE_ENABLED=true`, SLAI calls OmniRoute `CreateAPIKey` and uses the raw key returned by OmniRoute as the user-visible key.

## Usage Ingestion

SLAI bills usage asynchronously from OmniRoute logs. Users still call OmniRoute `/v1/*` directly with the OmniRoute-generated key; SLAI maps OmniRoute key ids back to local `api_keys.omniroute_key_id`, writes a `usage_events` row, and deducts credits through the ledger in the same database transaction.

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

`POST /v1/admin/usage/sync` reads `OMNIROUTE_USAGE_SYNC_MODE` and calls the OmniRoute interface for call logs or usage history. The real OmniRoute HTTP client is still pending, so the built-in stub returns a clean `501` until that client is implemented.

## OmniRoute Requirement

SLAI creates/manages OmniRoute API keys and syncs OmniRoute call logs or usage history to deduct credits. For server-to-server management and usage sync, OmniRoute likely needs a small patch:

- Add `OMNIROUTE_MANAGEMENT_TOKEN`
- Allow `Authorization: Bearer <token>` on OmniRoute management APIs such as `/api/keys`

The SLAI API currently includes the key-management and usage-sync abstractions but intentionally uses a stub client until OmniRoute management auth and the real HTTP client are available.

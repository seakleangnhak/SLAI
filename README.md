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
- Database guard preventing direct `credit_balances` mutation outside the ledger service transaction path
- Admin seed command: `slai-api seed-admin`
- OmniRoute client interface and stub client
- Next.js app shells for landing, login, user dashboard, and admin dashboard
- Docker Compose for Postgres, migrations, API, and web

Not implemented yet:

- Stripe or external payments
- API key creation
- OmniRoute usage ingestion
- Real OmniRoute management API calls

## API Surface

- `POST /v1/auth/signup`
- `POST /v1/auth/login`
- `POST /v1/auth/logout`
- `GET /v1/me`
- `GET /v1/packages`
- `GET /v1/balance`
- `GET /v1/ledger`
- `GET /v1/admin/packages`
- `POST /v1/admin/packages`
- `PATCH /v1/admin/packages/{id}`
- `POST /v1/admin/payments/manual-topup`
- `POST /v1/admin/ledger/adjustments`

## OmniRoute Requirement

SLAI will create/manage OmniRoute API keys and sync OmniRoute call logs or usage history to deduct credits. For server-to-server management, OmniRoute likely needs a small patch:

- Add `OMNIROUTE_MANAGEMENT_TOKEN`
- Allow `Authorization: Bearer <token>` on OmniRoute management APIs such as `/api/keys`

The SLAI API currently includes the abstraction for this integration but intentionally uses a stub client until OmniRoute management auth is available.

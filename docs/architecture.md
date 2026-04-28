# SLAI Architecture Foundation

SLAI is the business layer for prepaid AI credits. OmniRoute is the gateway layer. Users call
OmniRoute `/v1/*` directly with an OmniRoute-generated key that SLAI creates and owns on their
behalf.

## Boundaries

- OmniRoute: AI gateway, provider routing, model calls, usage/call logs.
- SLAI: users, credits, balances, manual top-ups, API key ownership, usage billing, admin
  operations, audit logs.

## MVP Rules

- One active API key per user.
- Manual admin-created top-ups only.
- Credits never expire.
- Credits and money use integer units only.
- Every balance change goes through `credit_ledger_entries`.
- Usage ingestion is idempotent through `UNIQUE (external_source, external_event_id)` and ledger
  idempotency keys.
- Raw API keys are never stored. Store `key_hash` and `key_prefix` only.

## OmniRoute Integration

The Go package `internal/omniroute` defines the management and usage-sync client interface. SLAI has
both a real HTTP client and a stub client:

- `OMNIROUTE_ENABLED=true`: SLAI uses the real HTTP client with `Authorization: Bearer
  <OMNIROUTE_MANAGEMENT_TOKEN>` for `/api/keys*` and `/api/usage/call-logs`.
- `OMNIROUTE_ENABLED=false`: SLAI uses local key generation and the stub client for local
  development.

SLAI creates, disables, enables, and deletes OmniRoute API keys through the management API. Usage
billing is asynchronous: SLAI syncs OmniRoute call logs, maps `apiKeyId` to
`api_keys.omniroute_key_id`, inserts idempotent usage events, and deducts credits through the
ledger.

## Sync Worker

Automatic usage sync is handled by a background worker when `USAGE_SYNC_WORKER_ENABLED=true`. Each
sync tick uses a PostgreSQL advisory lock so multiple API replicas do not process the same batch
concurrently. Manual admin sync remains available through `POST /v1/admin/usage/sync`, and status is
exposed through `GET /v1/admin/usage/sync-status`.

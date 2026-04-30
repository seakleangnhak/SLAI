# SLAI Architecture Foundation

SLAI is the business layer for prepaid AI credits. OmniRoute is the gateway layer. Users call
OmniRoute `/v1/*` directly with an OmniRoute-generated key that SLAI creates and owns on their
behalf.

## Boundaries

- OmniRoute: AI gateway, provider routing, model calls, usage/call logs.
- SLAI: users, credits, balances, package payments, manual admin top-ups, API key ownership, usage billing, admin
  operations, audit logs.

## MVP Rules

- One active API key per user.
- Users can create package checkouts through slai-payment; admins can still create manual top-ups.
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

## Bakong KHQR Payments

Bakong KHQR package checkout is delegated to the `slai-payment` service. SLAI creates a local `pending_payment` row, calls `POST /api/payments` on slai-payment, stores the returned payment id/reference/QR payload, and shows the provider-generated KHQR on `/checkout/{payment_id}`.

Payment confirmation is callback-driven. slai-payment signs the exact callback JSON body with `HMAC-SHA256(secret, timestamp + "." + raw_body)` and sends `X-SLAI-Payment-Timestamp`, `X-SLAI-Payment-Signature`, and `X-SLAI-Payment-ID`. SLAI verifies the signature and timestamp tolerance before trusting the payload.

A provider `PAID` status only credits the balance after SLAI validates payment id/reference, amount, and currency. Crediting runs in a database transaction and uses ledger idempotency key `slai_payment_paid:{payment_id}`, so duplicate callbacks or manual refreshes cannot double-credit. Provider transaction ids are unique for paid Bakong payments when present; conflicts move the payment to `needs_review`.

Legacy manual proof tables and endpoints remain for old records and fallback operation. Stored files live under `STORAGE_DIR/payment-settings` and `STORAGE_DIR/payment-proofs`; APIs return controlled file endpoints, never raw local paths.

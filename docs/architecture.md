# SLAI Architecture Foundation

SLAI is the business layer for prepaid AI credits. OmniRoute is the gateway layer. Users call OmniRoute `/v1/*` directly with an OmniRoute-generated key that SLAI creates and owns on their behalf.

## Boundaries

- OmniRoute: AI gateway, provider routing, model calls, usage/call logs.
- SLAI: users, credits, balances, manual top-ups, API key ownership, usage billing, admin operations, audit logs.

## MVP Rules

- One active API key per user.
- Manual admin-created top-ups only.
- Credits never expire.
- Credits and money use integer units only.
- Every balance change goes through `credit_ledger_entries`.
- Usage ingestion is idempotent through `UNIQUE (external_source, external_event_id)` and ledger idempotency keys.
- Raw API keys are never stored. Store `key_hash` and `key_prefix` only.

## OmniRoute Integration

The Go package `internal/omniroute` defines the client interface. The current implementation is a stub. Real calls should be added only after OmniRoute exposes server-to-server management auth with `OMNIROUTE_MANAGEMENT_TOKEN`.

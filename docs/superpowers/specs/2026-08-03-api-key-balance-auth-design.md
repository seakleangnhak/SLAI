# API-Key Authentication for Balance Endpoint

## Goal

Allow a user to read their available SLAI credits through the existing
`GET /v1/balance` endpoint by presenting either their SLAI session cookie or
their SLAI API key.

## Scope

This change applies only to `GET /v1/balance`. Other account endpoints remain
session-authenticated. The response schema and credit-unit representation do
not change. No database migration is required.

## Authentication Behavior

The endpoint supports two authentication methods:

1. `Authorization: Bearer <SLAI_API_KEY>`
2. The existing `slai_session` cookie

If an `Authorization` header is present, Bearer authentication takes
precedence. An invalid or unsupported Authorization value returns
`401 unauthenticated`; the handler does not fall back to a session cookie.
When no Authorization header is present, the endpoint uses the existing
session-cookie authentication.

API-key authentication computes the existing peppered HMAC hash of the raw key
and looks up the matching database row. The raw key is never persisted or
logged.

An API key is accepted when all of these conditions hold:

- Its status is `ACTIVE` or `SUSPENDED`.
- Its owner has `ACTIVE` user status.
- Its stored hash matches the presented key.

Suspended keys retain balance-read access so users can confirm an exhausted or
negative balance. Revoked and unknown keys are rejected. All API-key
authentication failures return the same generic `401 unauthenticated` response
so the endpoint does not reveal whether a key exists or why it was rejected.

## Components

The API-key service gains a read-only authentication operation that returns the
key owner identity for a matching usable key. It reuses the existing key hash
configuration and database index.

The HTTP server gains a balance-specific authentication helper. It selects
Bearer authentication when the Authorization header is present and otherwise
delegates to the existing session authentication. The balance handler then
loads the ledger balance using the authenticated user ID exactly as it does
today.

This endpoint does not update `api_keys.last_used_at`. Reading a balance remains
a read-only operation, and this change does not redefine existing key-usage
tracking semantics.

## API Contract

Example request:

```http
GET /v1/balance HTTP/1.1
Authorization: Bearer sk_slai_example
```

The successful response remains unchanged:

```json
{
  "balance": {
    "userId": "user-id",
    "availableUnits": 12500000,
    "lifetimePurchasedUnits": 20000000,
    "lifetimeUsedUnits": 7500000,
    "version": 4,
    "updatedAt": "2026-08-03T00:00:00Z"
  }
}
```

Credit values continue to use integer micro-units, where `1 credit` equals
`1,000,000 units`.

## Error Handling

- Missing credentials return `401 unauthenticated`.
- Invalid Bearer syntax or an invalid key returns `401 unauthenticated`.
- Revoked keys return `401 unauthenticated`.
- Keys owned by suspended users return `401 unauthenticated`.
- Balance lookup failures retain the existing `500 balance_failed` response.

Authentication errors do not include the raw key or disclose key status.

## Verification

HTTP tests will verify:

- An existing valid session can still read the balance.
- An active API key reads only its owner's balance.
- A suspended API key can read its owner's balance.
- Revoked, unknown, and malformed API keys receive `401`.
- A suspended user's API key receives `401`.
- An explicit invalid Authorization header is not rescued by a valid cookie.
- The successful JSON response schema is unchanged.

The API documentation will show both cookie and Bearer authentication for the
balance endpoint.

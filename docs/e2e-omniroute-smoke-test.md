# SLAI + OmniRoute E2E Smoke Test

This guide verifies the full prepaid billing flow against the patched OmniRoute
fork:

1. SLAI creates an OmniRoute API key.
2. A user calls OmniRoute `/v1/*` directly with that key.
3. SLAI syncs OmniRoute call logs.
4. SLAI deducts credits once per external usage event.
5. SLAI suspends the key when balance reaches zero or below.

## Prerequisites

- Patched OmniRoute fork: https://github.com/seakleangnhak/OmniRoute
- PostgreSQL for SLAI
- SLAI API running from this repository
- `curl` and `jq`
- An OmniRoute model/provider configuration that can answer
  `/v1/chat/completions`

Use one shared management token for OmniRoute and SLAI. Do not commit this
value.

## 1. Start Patched OmniRoute

Set OmniRoute environment variables before starting the patched fork:

```sh
export REQUIRE_API_KEY=true
export ALLOW_API_KEY_REVEAL=false
export OMNIROUTE_MANAGEMENT_TOKEN=<long-random-secret>
```

Start OmniRoute using the fork's normal local command. Record its base URL:

```sh
export OMNIROUTE_BASE_URL=http://localhost:4000
```

## 2. Start SLAI With OmniRoute Enabled

Set SLAI environment variables. `OMNIROUTE_MANAGEMENT_TOKEN` must match
OmniRoute.

```sh
export SLAI_API_URL=http://localhost:8080
export DATABASE_URL=postgres://slai:slai@localhost:5432/slai?sslmode=disable
export OMNIROUTE_ENABLED=true
export OMNIROUTE_BASE_URL=http://localhost:4000
export OMNIROUTE_MANAGEMENT_TOKEN=<same-secret>
export OMNIROUTE_USAGE_SYNC_MODE=call_logs
export OMNIROUTE_HTTP_TIMEOUT_SECONDS=15
export OMNIROUTE_CALL_LOG_LIMIT=100
export USAGE_SYNC_WORKER_ENABLED=true
export USAGE_SYNC_INTERVAL_SECONDS=60
export USAGE_SYNC_START_DELAY_SECONDS=10
```

Run SLAI migrations:

```sh
cd services/api
go run ./cmd/slai-api migrate up
```

Seed an admin:

```sh
ADMIN_SEED_EMAIL=admin@example.com \
ADMIN_SEED_PASSWORD=change-me-admin-password \
go run ./cmd/slai-api seed-admin
```

Start the SLAI API:

```sh
go run ./cmd/slai-api serve
```

In another shell, verify readiness:

```sh
curl -sS "$SLAI_API_URL/healthz"
curl -sS "$SLAI_API_URL/readyz" | jq .
```

## 3. Login Admin

Use cookie jars for authenticated requests:

```sh
export ADMIN_COOKIE=/tmp/slai-admin.cookies
export USER_COOKIE=/tmp/slai-user.cookies
export SLAI_ADMIN_EMAIL=admin@example.com
export SLAI_ADMIN_PASSWORD=change-me-admin-password
export SLAI_USER_EMAIL=smoke-user@example.com
export SLAI_USER_PASSWORD=change-me-user-password
```

Login as admin:

```sh
curl -sS -c "$ADMIN_COOKIE" \
  -H 'Content-Type: application/json' \
  -d '{"email":"'"$SLAI_ADMIN_EMAIL"'","password":"'"$SLAI_ADMIN_PASSWORD"'"}' \
  "$SLAI_API_URL/v1/auth/login" | jq .
```

## 4. Create a Normal User

Create a user:

```sh
curl -sS -c "$USER_COOKIE" \
  -H 'Content-Type: application/json' \
  -d '{"email":"'"$SLAI_USER_EMAIL"'","password":"'"$SLAI_USER_PASSWORD"'"}' \
  "$SLAI_API_URL/v1/auth/signup" | tee /tmp/slai-user.json | jq .

export SLAI_USER_ID=$(jq -r '.user.id' /tmp/slai-user.json)
```

If the user already exists, login instead and read `/v1/me`:

```sh
curl -sS -c "$USER_COOKIE" \
  -H 'Content-Type: application/json' \
  -d '{"email":"'"$SLAI_USER_EMAIL"'","password":"'"$SLAI_USER_PASSWORD"'"}' \
  "$SLAI_API_URL/v1/auth/login" | jq .

curl -sS -b "$USER_COOKIE" \
  "$SLAI_API_URL/v1/me" | tee /tmp/slai-user.json | jq .

export SLAI_USER_ID=$(jq -r '.user.id' /tmp/slai-user.json)
```

## 5. Admin Creates a Credit Package if Needed

Packages are not required for manual top-up, but this verifies the admin package
path:

```sh
curl -sS -b "$ADMIN_COOKIE" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Smoke Starter",
    "description": "Smoke test credits",
    "creditUnits": 10000,
    "bonusCreditUnits": 0,
    "priceMinor": 1000,
    "currency": "USD",
    "active": true,
    "sortOrder": 10
  }' \
  "$SLAI_API_URL/v1/admin/packages" | jq .
```

## 6. Admin Manually Tops Up the User

```sh
curl -sS -b "$ADMIN_COOKIE" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: smoke-topup-001' \
  -d '{
    "userId": "'"$SLAI_USER_ID"'",
    "amountMinor": 1000,
    "currency": "USD",
    "creditUnits": 10000,
    "note": "E2E smoke top-up"
  }' \
  "$SLAI_API_URL/v1/admin/payments/manual-topup" | jq .
```

Check balance:

```sh
curl -sS -b "$USER_COOKIE" \
  "$SLAI_API_URL/v1/balance" | jq .
```

## 7. User Creates an API Key

```sh
curl -sS -b "$USER_COOKIE" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Smoke key"}' \
  "$SLAI_API_URL/v1/api-key" | tee /tmp/slai-api-key.json | jq .

export SLAI_OMNIROUTE_KEY=$(jq -r '.raw_api_key' /tmp/slai-api-key.json)
export SLAI_API_KEY_ID=$(jq -r '.api_key.id' /tmp/slai-api-key.json)
```

The raw API key is shown only once. SLAI stores only hash plus display prefix.

## 8. Confirm SLAI Created an OmniRoute Key

Confirm SLAI linked the user key to an OmniRoute key ID:

```sh
curl -sS -b "$ADMIN_COOKIE" \
  "$SLAI_API_URL/v1/admin/users/$SLAI_USER_ID/api-key" | jq .
```

Optional: if you have the management token in this shell, confirm the key exists
in OmniRoute:

```sh
curl -sS \
  -H "Authorization: Bearer $OMNIROUTE_MANAGEMENT_TOKEN" \
  "$OMNIROUTE_BASE_URL/api/keys" | jq .
```

## 9. Call OmniRoute With the Generated Key

Use a model configured in OmniRoute:

```sh
export OMNIROUTE_MODEL=${OMNIROUTE_MODEL:-gpt-4o-mini}

curl -sS \
  -H "Authorization: Bearer $SLAI_OMNIROUTE_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "'"$OMNIROUTE_MODEL"'",
    "messages": [
      {
        "role": "user",
        "content": "Reply with one short sentence for a SLAI smoke test."
      }
    ],
    "max_tokens": 32
  }' \
  "$OMNIROUTE_BASE_URL/v1/chat/completions" | jq .
```

## 10. Trigger Manual Usage Sync

```sh
curl -sS -b "$ADMIN_COOKIE" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{}' \
  "$SLAI_API_URL/v1/admin/usage/sync" | jq .
```

## 11. Check Usage, Balance, and Ledger

```sh
curl -sS -b "$USER_COOKIE" \
  "$SLAI_API_URL/v1/usage?limit=10" | jq .

curl -sS -b "$USER_COOKIE" \
  "$SLAI_API_URL/v1/balance" | jq .

curl -sS -b "$USER_COOKIE" \
  "$SLAI_API_URL/v1/ledger" | jq .
```

Expected result:

- A usage event has `status` set to `billed`.
- Balance is reduced.
- Ledger contains a `usage_debit` entry with a negative `deltaUnits` value.

## 12. Confirm Duplicate Sync Does Not Deduct Again

Run manual sync again:

```sh
curl -sS -b "$ADMIN_COOKIE" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{}' \
  "$SLAI_API_URL/v1/admin/usage/sync" | jq .
```

Expected result:

- Duplicate events are reported or ignored by idempotency.
- Balance does not decrease for the same OmniRoute call log again.

## 13. Drive Balance to Zero or Below

For a deterministic test, create a fresh user with a small top-up or repeatedly
call OmniRoute until usage costs exceed the available balance. Then sync again:

```sh
for i in $(seq 1 20); do
  curl -sS \
    -H "Authorization: Bearer $SLAI_OMNIROUTE_KEY" \
    -H 'Content-Type: application/json' \
    -d '{
      "model": "'"$OMNIROUTE_MODEL"'",
      "messages": [
        {
          "role": "user",
          "content": "Generate a longer smoke-test response so usage is billed."
        }
      ],
      "max_tokens": 128
    }' \
    "$OMNIROUTE_BASE_URL/v1/chat/completions" >/dev/null
done

curl -sS -b "$ADMIN_COOKIE" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{}' \
  "$SLAI_API_URL/v1/admin/usage/sync" | jq .
```

## 14. Confirm Key Suspension

Check local key status:

```sh
curl -sS -b "$ADMIN_COOKIE" \
  "$SLAI_API_URL/v1/admin/users/$SLAI_USER_ID/api-key" | jq .
```

Check OmniRoute key status if the management token is available:

```sh
curl -sS \
  -H "Authorization: Bearer $OMNIROUTE_MANAGEMENT_TOKEN" \
  "$OMNIROUTE_BASE_URL/api/keys" | jq .
```

Expected result:

- SLAI marks the key `SUSPENDED`.
- OmniRoute has the matching key disabled with `isActive=false` or equivalent
  status.

## 15. Check Sync Status

```sh
curl -sS -b "$ADMIN_COOKIE" \
  "$SLAI_API_URL/v1/admin/usage/sync-status" | jq .
```

Expected fields:

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

## Optional Helper Script

The repository includes a helper for the common path:

```sh
SLAI_API_URL=http://localhost:8080 \
SLAI_ADMIN_EMAIL=admin@example.com \
SLAI_ADMIN_PASSWORD=change-me-admin-password \
SLAI_USER_EMAIL=smoke-user@example.com \
SLAI_USER_PASSWORD=change-me-user-password \
OMNIROUTE_BASE_URL=http://localhost:4000 \
scripts/smoke-slai-omniroute.sh
```

Optional script variables:

- `OMNIROUTE_MODEL`, default `gpt-4o-mini`
- `SLAI_TOPUP_CREDIT_UNITS`, default `10000`
- `SLAI_TOPUP_AMOUNT_MINOR`, default `1000`
- `SLAI_ROTATE_EXISTING_KEY=true` to rotate if the user already has an active
  key
- `SLAI_EXHAUST_BALANCE=true` to run repeated OmniRoute calls and sync again
- `SLAI_EXHAUST_REQUESTS`, default `20`

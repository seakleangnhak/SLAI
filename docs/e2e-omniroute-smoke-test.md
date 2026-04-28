# SLAI + OmniRoute E2E Smoke Test

This guide verifies the full prepaid billing flow against the patched OmniRoute
fork.

The flow proves that:

1. SLAI creates an OmniRoute API key.
2. A user calls OmniRoute `/v1/*` directly with that key.
3. SLAI syncs OmniRoute call logs.
4. SLAI deducts credits once per external usage event.
5. SLAI does not double-deduct duplicate usage logs.
6. SLAI suspends the API key when balance reaches zero or below.
7. OmniRoute receives the key disable request when integration is enabled.

## Prerequisites

Install or start:

- Patched OmniRoute fork
- SLAI from this repository
- PostgreSQL for SLAI
- `curl`
- `jq`

Patched OmniRoute fork:

```text
https://github.com/seakleangnhak/OmniRoute
```

You also need an OmniRoute model and provider configuration that can answer the
OpenAI-compatible endpoint:

```text
/v1/chat/completions
```

Use one shared management token for OmniRoute and SLAI.
Do not commit this token.
Do not paste it in issue comments or logs.

## Variables Used In This Guide

Set these shell variables in the terminal where you run the examples:

```sh
export SLAI_API_URL=http://localhost:8080
export OMNIROUTE_BASE_URL=http://localhost:4000
export SLAI_ADMIN_EMAIL=admin@example.com
export SLAI_ADMIN_PASSWORD=change-me-admin-password
export SLAI_USER_EMAIL=developer@example.com
export SLAI_USER_PASSWORD=change-me-user-password
export OMNIROUTE_SMOKE_MODEL=gpt-4o-mini
```

Set the management token in both services:

```sh
export OMNIROUTE_MANAGEMENT_TOKEN=<long-random-secret>
```

The token value must be identical in OmniRoute and SLAI.

## 1. Start Patched OmniRoute

Clone and start the patched OmniRoute fork using its normal local workflow.
Before starting OmniRoute, set:

```sh
export REQUIRE_API_KEY=true
export ALLOW_API_KEY_REVEAL=false
export OMNIROUTE_MANAGEMENT_TOKEN=<long-random-secret>
```

Record the base URL:

```sh
export OMNIROUTE_BASE_URL=http://localhost:4000
```

Verify OmniRoute is reachable:

```sh
curl -sS "$OMNIROUTE_BASE_URL" || true
```

The root response can vary by OmniRoute app version.
The important part is that the server is reachable.

## 2. Start SLAI With OmniRoute Enabled

Set SLAI environment variables:

```sh
export APP_ENV=development
export HTTP_ADDR=:8080
export DATABASE_URL=postgres://slai:slai@localhost:5432/slai?sslmode=disable
export MIGRATIONS_DIR=../../db/migrations
export LOG_LEVEL=info
export SESSION_SECRET=dev-only-change-me
export COOKIE_SECURE=false
export SESSION_TTL=168h
export API_KEY_PEPPER=dev-only-change-me-api-key-pepper
export API_KEY_PREFIX=sk_slai
export READINESS_TIMEOUT=2s
export SHUTDOWN_TIMEOUT=10s
```

Enable OmniRoute integration:

```sh
export OMNIROUTE_ENABLED=true
export OMNIROUTE_BASE_URL=http://localhost:4000
export OMNIROUTE_MANAGEMENT_TOKEN=<same-secret>
export OMNIROUTE_USAGE_SYNC_MODE=call_logs
export OMNIROUTE_HTTP_TIMEOUT_SECONDS=15
export OMNIROUTE_CALL_LOG_LIMIT=100
```

Enable the scheduled usage sync worker if you want automatic sync during the
smoke test:

```sh
export USAGE_SYNC_WORKER_ENABLED=true
export USAGE_SYNC_INTERVAL_SECONDS=60
export USAGE_SYNC_LOCK_KEY=slai_usage_sync
export USAGE_SYNC_BATCH_LIMIT=100
export USAGE_SYNC_START_DELAY_SECONDS=10
```

Manual sync is still available even when the worker is enabled.

## 3. Run Migrations

From the SLAI repository root:

```sh
cd services/api
go run ./cmd/slai-api migrate up
```

Return to the repository root if desired:

```sh
cd ../..
```

## 4. Seed An Admin

From `services/api`:

```sh
ADMIN_SEED_EMAIL="$SLAI_ADMIN_EMAIL" \
ADMIN_SEED_PASSWORD="$SLAI_ADMIN_PASSWORD" \
go run ./cmd/slai-api seed-admin
```

The seed command is idempotent for the same admin email.

## 5. Start The SLAI API

From `services/api`:

```sh
go run ./cmd/slai-api serve
```

In another shell, verify health:

```sh
curl -sS "$SLAI_API_URL/healthz"
```

Expected response:

```text
OK
```

Verify readiness:

```sh
curl -sS "$SLAI_API_URL/readyz" | jq .
```

Expected shape:

```json
{
  "status": "ready"
}
```

## 6. Create Cookie Jars

The API uses HttpOnly session cookies.
Use cookie jar files for curl examples:

```sh
export ADMIN_COOKIES=/tmp/slai-admin.cookies
export USER_COOKIES=/tmp/slai-user.cookies
rm -f "$ADMIN_COOKIES" "$USER_COOKIES"
```

## 7. Log In As Admin

```sh
curl -sS -X POST "$SLAI_API_URL/v1/auth/login" \
  -c "$ADMIN_COOKIES" \
  -b "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -d "{
    \"email\": \"$SLAI_ADMIN_EMAIL\",
    \"password\": \"$SLAI_ADMIN_PASSWORD\"
  }" | jq .
```

Confirm the admin session:

```sh
curl -sS "$SLAI_API_URL/v1/me" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | jq .
```

The user role should be `ADMIN`.

## 8. Create A Normal User

```sh
curl -sS -X POST "$SLAI_API_URL/v1/auth/signup" \
  -c "$USER_COOKIES" \
  -b "$USER_COOKIES" \
  -H 'Content-Type: application/json' \
  -d "{
    \"email\": \"$SLAI_USER_EMAIL\",
    \"password\": \"$SLAI_USER_PASSWORD\"
  }" | tee /tmp/slai-user-signup.json | jq .
```

If the user already exists, log in instead:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/auth/login" \
  -c "$USER_COOKIES" \
  -b "$USER_COOKIES" \
  -H 'Content-Type: application/json' \
  -d "{
    \"email\": \"$SLAI_USER_EMAIL\",
    \"password\": \"$SLAI_USER_PASSWORD\"
  }" | tee /tmp/slai-user-login.json | jq .
```

Capture the user ID:

```sh
export SLAI_USER_ID=$(
  curl -sS "$SLAI_API_URL/v1/me" \
    -b "$USER_COOKIES" \
    -c "$USER_COOKIES" | jq -r '.user.id'
)
```

Check it:

```sh
printf 'SLAI user ID: %s\n' "$SLAI_USER_ID"
```

## 9. Create A Credit Package

If a package already exists, this step is optional.
Creating one keeps the smoke test self-contained.

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/packages" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Smoke Test Pack",
    "description": "Temporary package for E2E testing",
    "creditUnits": 1000,
    "bonusCreditUnits": 0,
    "priceMinor": 1000,
    "currency": "USD",
    "active": true,
    "sortOrder": 10
  }' | tee /tmp/slai-package.json | jq .
```

Capture the package ID:

```sh
export SLAI_PACKAGE_ID=$(jq -r '.package.id' /tmp/slai-package.json)
```

## 10. Top Up The User

Manual top-ups are admin-only.
They create a payment row, a ledger entry, a balance update, and an audit log.

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/payments/manual-topup" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: smoke-topup-001' \
  -d "{
    \"userId\": \"$SLAI_USER_ID\",
    \"packageId\": \"$SLAI_PACKAGE_ID\",
    \"amountMinor\": 1000,
    \"currency\": \"USD\",
    \"creditUnits\": 1000,
    \"note\": \"E2E smoke test top-up\"
  }" | tee /tmp/slai-topup.json | jq .
```

Check user balance:

```sh
curl -sS "$SLAI_API_URL/v1/balance" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

The available balance should be positive.

## 11. User Creates An API Key

```sh
curl -sS -X POST "$SLAI_API_URL/v1/api-key" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Smoke Test Key"}' \
  | tee /tmp/slai-api-key-created.json | jq .
```

Capture the raw key immediately.
It is shown only once.

```sh
export SLAI_RAW_API_KEY=$(jq -r '.raw_api_key' /tmp/slai-api-key-created.json)
export SLAI_API_KEY_ID=$(jq -r '.api_key.id' /tmp/slai-api-key-created.json)
```

Do not store the raw key permanently.
Do not commit it.
Use it only for this local smoke test.

## 12. Confirm SLAI Linked The OmniRoute Key

As the user:

```sh
curl -sS "$SLAI_API_URL/v1/api-key" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

Expected fields include:

- `api_key.id`
- `api_key.key_prefix`
- `api_key.status`
- `api_key.omniroute_linked`

As admin:

```sh
curl -sS "$SLAI_API_URL/v1/admin/users/$SLAI_USER_ID/api-key" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | jq .
```

Expected admin-only fields include:

- `api_key.omniroute_key_id`
- `api_key.status`

No endpoint should return the raw key after the create response.

## 13. Optionally Confirm Key In OmniRoute

If you have the management token in your shell, you can list OmniRoute keys:

```sh
curl -sS "$OMNIROUTE_BASE_URL/api/keys" \
  -H "Authorization: Bearer $OMNIROUTE_MANAGEMENT_TOKEN" | jq .
```

Do not print the management token.
The list endpoint should not reveal raw API keys.

## 14. Call OmniRoute With The User Key

Use the raw key created by SLAI to call OmniRoute directly:

```sh
curl -sS -X POST "$OMNIROUTE_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SLAI_RAW_API_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"model\": \"$OMNIROUTE_SMOKE_MODEL\",
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": \"Reply with the word SLAI.\"
      }
    ]
  }" | tee /tmp/slai-omniroute-chat.json | jq .
```

The exact response depends on your OmniRoute provider configuration.
The call should create a call log in OmniRoute.

## 15. Trigger Manual Usage Sync

Manual sync remains available even when the worker is enabled:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/usage/sync" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -d '{}' | tee /tmp/slai-sync-1.json | jq .
```

Expected shape:

```json
{
  "sync": {
    "fetched": 1,
    "billed": 1,
    "duplicate": 0,
    "ignored": 0,
    "failed": 0,
    "suspended_keys": 0
  }
}
```

Counts can be higher if OmniRoute already has recent call logs.

## 16. Check Usage Events

As the user:

```sh
curl -sS "$SLAI_API_URL/v1/usage?limit=20" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | tee /tmp/slai-usage-user.json | jq .
```

As admin:

```sh
curl -sS "$SLAI_API_URL/v1/admin/usage?limit=20" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | tee /tmp/slai-usage-admin.json | jq .
```

Usage events should include:

- `external_source`
- `external_event_id`
- `api_key_id`
- `model`
- `provider`
- `input_tokens`
- `output_tokens`
- `cost_units`
- `status`

They should not include raw API keys.

## 17. Check Balance And Ledger

Check the updated balance:

```sh
curl -sS "$SLAI_API_URL/v1/balance" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | tee /tmp/slai-balance-after-sync.json | jq .
```

Check ledger entries:

```sh
curl -sS "$SLAI_API_URL/v1/ledger?limit=20" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | tee /tmp/slai-ledger-after-sync.json | jq .
```

Expected ledger behavior:

- The top-up created a positive `payment_credit` entry.
- Usage created a negative `usage_debit` entry.
- `lifetime_used_units` increased by the billed usage cost.
- `available_units` decreased by the billed usage cost.

## 18. Repeat Sync To Confirm Idempotency

Run manual sync again:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/usage/sync" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -d '{}' | tee /tmp/slai-sync-2.json | jq .
```

Check the balance again:

```sh
curl -sS "$SLAI_API_URL/v1/balance" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | tee /tmp/slai-balance-after-duplicate-sync.json | jq .
```

Expected result:

- Duplicate call logs do not create another ledger entry.
- Duplicate call logs do not deduct credits again.
- The sync result can show duplicates if OmniRoute returned prior logs.

## 19. Drive Balance To Zero Or Below

Use either of these methods.

Method A: create more OmniRoute usage until the balance is exhausted.
This is the closest production-like test, but it depends on provider cost and
model behavior.

Method B: use an admin adjustment to reduce the balance before another usage
call:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/ledger/adjustments" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: smoke-adjust-down-001' \
  -d "{
    \"userId\": \"$SLAI_USER_ID\",
    \"deltaUnits\": -999,
    \"reason\": \"Prepare E2E smoke test key suspension\"
  }" | jq .
```

Then make another OmniRoute call:

```sh
curl -sS -X POST "$OMNIROUTE_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SLAI_RAW_API_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"model\": \"$OMNIROUTE_SMOKE_MODEL\",
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": \"Reply with a short sentence for billing.\"
      }
    ]
  }" | tee /tmp/slai-omniroute-chat-exhaust.json | jq .
```

Sync again:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/admin/usage/sync" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -d '{}' | tee /tmp/slai-sync-exhaust.json | jq .
```

## 20. Confirm Key Suspension

Check the user key:

```sh
curl -sS "$SLAI_API_URL/v1/api-key" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

Check the admin view:

```sh
curl -sS "$SLAI_API_URL/v1/admin/users/$SLAI_USER_ID/api-key" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | jq .
```

Expected result:

```text
status = SUSPENDED
```

If `OMNIROUTE_ENABLED=true`, SLAI should also have sent:

```json
{
  "isActive": false
}
```

to OmniRoute:

```text
PATCH /api/keys/{id}
```

## 21. Confirm OmniRoute Rejects Or Blocks The Suspended Key

Behavior depends on OmniRoute version and route implementation, but a suspended
key should not be allowed to keep making successful `/v1/*` calls.

Try the call again:

```sh
curl -sS -i -X POST "$OMNIROUTE_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $SLAI_RAW_API_KEY" \
  -H 'Content-Type: application/json' \
  -d "{
    \"model\": \"$OMNIROUTE_SMOKE_MODEL\",
    \"messages\": [
      {
        \"role\": \"user\",
        \"content\": \"This should fail after suspension.\"
      }
    ]
  }"
```

Expected result is an authorization or disabled-key response.

## 22. Check Sync Status

```sh
curl -sS "$SLAI_API_URL/v1/admin/usage/sync-status" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | jq .
```

Expected fields:

- `worker_enabled`
- `last_started_at`
- `last_finished_at`
- `last_success_at`
- `last_error`
- `last_result`
- `next_run_at`
- `currently_running`

Manual sync and automatic worker sync both update this status.

## 23. Validate Raw Key Handling

Run these checks:

```sh
curl -sS "$SLAI_API_URL/v1/api-key" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

```sh
curl -sS "$SLAI_API_URL/v1/admin/users/$SLAI_USER_ID/api-key" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" | jq .
```

Expected result:

- User endpoint returns display metadata only.
- Admin endpoint returns display metadata and OmniRoute key ID.
- Neither endpoint returns the raw API key.
- The raw key appears only in the original create or rotate response.

## 24. Validate Ledger Invariants

Review ledger entries:

```sh
curl -sS "$SLAI_API_URL/v1/ledger?limit=50" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

Expected result:

- Every balance change has a ledger entry.
- Usage debits have negative `deltaUnits`.
- Top-ups have positive `deltaUnits`.
- Admin adjustments include a reason.
- `balanceAfterUnits` reflects each mutation order.

## 25. Optional Local Mock Event Check

If you want to test billing without OmniRoute, use local mode or the mock usage
endpoint.

The endpoint requires an admin session:

```sh
curl -sS -X POST "$SLAI_API_URL/v1/internal/usage/mock-event" \
  -b "$ADMIN_COOKIES" \
  -c "$ADMIN_COOKIES" \
  -H 'Content-Type: application/json' \
  -d "{
    \"api_key_id\": \"$SLAI_API_KEY_ID\",
    \"external_event_id\": \"mock-001\",
    \"model\": \"gpt-5.5\",
    \"provider\": \"openai\",
    \"input_tokens\": 7240,
    \"output_tokens\": 357,
    \"occurred_at\": \"2026-04-28T10:00:00Z\"
  }" | jq .
```

Repeat the same request and confirm it is duplicate-safe.

## Troubleshooting

If `POST /v1/api-key` fails:

- Confirm `OMNIROUTE_ENABLED=true` in SLAI.
- Confirm `OMNIROUTE_BASE_URL` points to the patched OmniRoute server.
- Confirm `OMNIROUTE_MANAGEMENT_TOKEN` matches in both services.
- Confirm OmniRoute accepts `Authorization: Bearer <token>` on `/api/keys`.

If usage sync returns 501:

- Confirm SLAI is using the real OmniRoute HTTP client.
- Confirm `OMNIROUTE_ENABLED=true`.
- Confirm `OMNIROUTE_USAGE_SYNC_MODE=call_logs`.

If sync returns zero fetched events:

- Confirm the OmniRoute call completed successfully.
- Confirm OmniRoute wrote a call log.
- Confirm SLAI selected `/api/usage/call-logs`.
- Try increasing `OMNIROUTE_CALL_LOG_LIMIT`.
- Trigger manual sync again.

If a usage event is ignored:

- Confirm the call log contains `apiKeyId`.
- Confirm `apiKeyId` matches `api_keys.omniroute_key_id` in SLAI.
- Confirm the API key was created through SLAI.

If duplicate usage deducts twice:

- Stop and inspect `usage_events`.
- The pair `external_source` plus `external_event_id` must be unique.
- The ledger idempotency key should be `usage:{source}:{event_id}`.

If the worker does not run:

- Confirm `USAGE_SYNC_WORKER_ENABLED=true`.
- Confirm the API process was restarted after changing env.
- Check `GET /v1/admin/usage/sync-status`.
- Check API logs for lock-held or sync-failed messages.

## Cleanup

Remove local cookie and response files:

```sh
rm -f /tmp/slai-admin.cookies
rm -f /tmp/slai-user.cookies
rm -f /tmp/slai-user-signup.json
rm -f /tmp/slai-user-login.json
rm -f /tmp/slai-package.json
rm -f /tmp/slai-topup.json
rm -f /tmp/slai-api-key-created.json
rm -f /tmp/slai-omniroute-chat.json
rm -f /tmp/slai-usage-user.json
rm -f /tmp/slai-usage-admin.json
rm -f /tmp/slai-balance-after-sync.json
rm -f /tmp/slai-ledger-after-sync.json
rm -f /tmp/slai-sync-1.json
rm -f /tmp/slai-sync-2.json
rm -f /tmp/slai-sync-exhaust.json
```

Revoke the smoke-test API key when finished:

```sh
curl -sS -X DELETE "$SLAI_API_URL/v1/api-key" \
  -b "$USER_COOKIES" \
  -c "$USER_COOKIES" | jq .
```

#!/usr/bin/env bash
set -euo pipefail

required_vars=(
  SLAI_API_URL
  SLAI_ADMIN_EMAIL
  SLAI_ADMIN_PASSWORD
  SLAI_USER_EMAIL
  SLAI_USER_PASSWORD
  OMNIROUTE_BASE_URL
)

missing=()
for name in "${required_vars[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    missing+=("$name")
  fi
done

if (( ${#missing[@]} > 0 )); then
  printf 'Missing required environment variables:\n' >&2
  printf '  %s\n' "${missing[@]}" >&2
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo 'curl is required' >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo 'jq is required' >&2
  exit 1
fi

SLAI_API_URL="${SLAI_API_URL%/}"
OMNIROUTE_BASE_URL="${OMNIROUTE_BASE_URL%/}"
OMNIROUTE_MODEL="${OMNIROUTE_MODEL:-gpt-4o-mini}"
SLAI_TOPUP_CREDIT_UNITS="${SLAI_TOPUP_CREDIT_UNITS:-10000}"
SLAI_TOPUP_AMOUNT_MINOR="${SLAI_TOPUP_AMOUNT_MINOR:-1000}"
SLAI_EXHAUST_REQUESTS="${SLAI_EXHAUST_REQUESTS:-20}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

admin_cookie="$tmpdir/admin.cookies"
user_cookie="$tmpdir/user.cookies"
response_file="$tmpdir/response.json"
RESPONSE_CODE=""
RESPONSE_BODY=""
raw_api_key=""

step() {
  printf '\n==> %s\n' "$1"
}

sanitize_body() {
  local body="$1"
  if [[ -n "${raw_api_key:-}" ]]; then
    body="${body//$raw_api_key/[redacted-api-key]}"
  fi
  printf '%s\n' "$body"
}

request() {
  local method="$1"
  local url="$2"
  local cookie_mode="$3"
  local cookie_file="$4"
  local data="${5:-}"
  shift 5 || true

  local args=(-sS -o "$response_file" -w '%{http_code}' -X "$method" "$url")

  case "$cookie_mode" in
    none) ;;
    jar) args+=(-c "$cookie_file") ;;
    send) args+=(-b "$cookie_file") ;;
    both) args+=(-b "$cookie_file" -c "$cookie_file") ;;
    *) echo "unknown cookie mode: $cookie_mode" >&2; exit 1 ;;
  esac

  if [[ -n "$data" ]]; then
    args+=(-H 'Content-Type: application/json' -d "$data")
  fi

  while (( $# > 0 )); do
    args+=(-H "$1")
    shift
  done

  RESPONSE_CODE="$(curl "${args[@]}")"
  RESPONSE_BODY="$(cat "$response_file")"
}

expect_2xx() {
  local label="$1"
  case "$RESPONSE_CODE" in
    2*) return 0 ;;
  esac

  printf '%s failed with HTTP %s\n' "$label" "$RESPONSE_CODE" >&2
  if [[ -n "$RESPONSE_BODY" ]]; then
    sanitize_body "$RESPONSE_BODY" | jq . >&2 || sanitize_body "$RESPONSE_BODY" >&2
  fi
  exit 1
}

print_json() {
  if [[ -n "$RESPONSE_BODY" ]]; then
    sanitize_body "$RESPONSE_BODY" | jq .
  fi
}

step 'Checking SLAI readiness'
request GET "$SLAI_API_URL/readyz" none '' ''
expect_2xx 'SLAI readiness check'
print_json

step 'Logging in admin'
admin_login_payload="$(jq -n \
  --arg email "$SLAI_ADMIN_EMAIL" \
  --arg password "$SLAI_ADMIN_PASSWORD" \
  '{email:$email,password:$password}')"
request POST "$SLAI_API_URL/v1/auth/login" jar "$admin_cookie" "$admin_login_payload"
expect_2xx 'admin login'
print_json

step 'Creating or logging in smoke user'
user_auth_payload="$(jq -n \
  --arg email "$SLAI_USER_EMAIL" \
  --arg password "$SLAI_USER_PASSWORD" \
  '{email:$email,password:$password}')"
request POST "$SLAI_API_URL/v1/auth/signup" jar "$user_cookie" "$user_auth_payload"
if [[ "$RESPONSE_CODE" != 2* ]]; then
  echo 'Signup did not succeed, trying login for existing user.'
  request POST "$SLAI_API_URL/v1/auth/login" jar "$user_cookie" "$user_auth_payload"
  expect_2xx 'user login'
fi
print_json

request GET "$SLAI_API_URL/v1/me" send "$user_cookie" ''
expect_2xx 'fetch current user'
user_id="$(printf '%s\n' "$RESPONSE_BODY" | jq -r '.user.id')"
printf 'Smoke user id: %s\n' "$user_id"

step 'Manual top-up user'
topup_payload="$(jq -n \
  --arg userId "$user_id" \
  --arg currency 'USD' \
  --arg note 'SLAI OmniRoute smoke top-up' \
  --argjson amountMinor "$SLAI_TOPUP_AMOUNT_MINOR" \
  --argjson creditUnits "$SLAI_TOPUP_CREDIT_UNITS" \
  '{userId:$userId,amountMinor:$amountMinor,currency:$currency,creditUnits:$creditUnits,note:$note}')"
request POST "$SLAI_API_URL/v1/admin/payments/manual-topup" \
  send "$admin_cookie" "$topup_payload" \
  "Idempotency-Key: smoke-topup-$(date +%s)"
expect_2xx 'manual top-up'
print_json

step 'Creating SLAI API key backed by OmniRoute'
key_payload="$(jq -n '{name:"Smoke key"}')"
request POST "$SLAI_API_URL/v1/api-key" send "$user_cookie" "$key_payload"
if [[ "$RESPONSE_CODE" != 2* ]]; then
  if [[ "${SLAI_ROTATE_EXISTING_KEY:-false}" == 'true' ]]; then
    echo 'Create failed, rotating existing key because SLAI_ROTATE_EXISTING_KEY=true.'
    request POST "$SLAI_API_URL/v1/api-key/rotate" send "$user_cookie" '{}'
  else
    printf 'API key creation failed with HTTP %s. Set SLAI_ROTATE_EXISTING_KEY=true to rotate.\n' \
      "$RESPONSE_CODE" >&2
    sanitize_body "$RESPONSE_BODY" | jq . >&2 || true
    exit 1
  fi
fi
expect_2xx 'api key create/rotate'
raw_api_key="$(printf '%s\n' "$RESPONSE_BODY" | jq -r '.raw_api_key')"
api_key_id="$(printf '%s\n' "$RESPONSE_BODY" | jq -r '.api_key.id')"
printf 'Created raw API key for this smoke run, shown once: %s\n' "$raw_api_key"
printf 'SLAI api key id: %s\n' "$api_key_id"

step 'Checking SLAI admin API key view'
request GET "$SLAI_API_URL/v1/admin/users/$user_id/api-key" send "$admin_cookie" ''
expect_2xx 'admin api key view'
print_json

step 'Calling OmniRoute /v1/chat/completions with the generated key'
chat_payload="$(jq -n \
  --arg model "$OMNIROUTE_MODEL" \
  '{
    model:$model,
    messages:[{role:"user",content:"Reply with one short sentence for a SLAI smoke test."}],
    max_tokens:32
  }')"
request POST "$OMNIROUTE_BASE_URL/v1/chat/completions" \
  none '' "$chat_payload" \
  "Authorization: Bearer $raw_api_key"
expect_2xx 'OmniRoute chat completion'
print_json

step 'Triggering manual usage sync'
request POST "$SLAI_API_URL/v1/admin/usage/sync" send "$admin_cookie" '{}'
expect_2xx 'manual usage sync'
print_json

step 'Checking user usage, balance, and ledger'
request GET "$SLAI_API_URL/v1/usage?limit=10" send "$user_cookie" ''
expect_2xx 'usage list'
print_json
request GET "$SLAI_API_URL/v1/balance" send "$user_cookie" ''
expect_2xx 'balance'
print_json
request GET "$SLAI_API_URL/v1/ledger" send "$user_cookie" ''
expect_2xx 'ledger'
print_json

step 'Repeating sync to confirm idempotency'
request POST "$SLAI_API_URL/v1/admin/usage/sync" send "$admin_cookie" '{}'
expect_2xx 'duplicate usage sync'
print_json

if [[ "${SLAI_EXHAUST_BALANCE:-false}" == 'true' ]]; then
  step "Generating additional usage to test suspension ($SLAI_EXHAUST_REQUESTS requests)"
  for _ in $(seq 1 "$SLAI_EXHAUST_REQUESTS"); do
    request POST "$OMNIROUTE_BASE_URL/v1/chat/completions" \
  none '' "$chat_payload" \
  "Authorization: Bearer $raw_api_key"
    expect_2xx 'OmniRoute exhaust request'
  done

  step 'Syncing after additional usage'
  request POST "$SLAI_API_URL/v1/admin/usage/sync" send "$admin_cookie" '{}'
  expect_2xx 'post-exhaust usage sync'
  print_json

  step 'Checking key status after potential suspension'
  request GET "$SLAI_API_URL/v1/admin/users/$user_id/api-key" send "$admin_cookie" ''
  expect_2xx 'admin api key view after exhaust'
  print_json
fi

step 'Checking usage sync status'
request GET "$SLAI_API_URL/v1/admin/usage/sync-status" send "$admin_cookie" ''
expect_2xx 'sync status'
print_json

step 'Smoke flow complete'

#!/usr/bin/env bash

set -Eeuo pipefail

# Smoke test for the SLAI + OmniRoute prepaid billing path.
#
# Required environment:
#   SLAI_ADMIN_EMAIL
#   SLAI_ADMIN_PASSWORD
#   SLAI_USER_EMAIL
#   SLAI_USER_PASSWORD
#   OMNIROUTE_BASE_URL
#
# Optional environment:
#   SLAI_API_URL
#   OMNIROUTE_MANAGEMENT_TOKEN
#   OMNIROUTE_SMOKE_MODEL
#   SLAI_SMOKE_LOAD_API_ENV
#   SLAI_SMOKE_PACKAGE_NAME
#   SLAI_SMOKE_PACKAGE_CREDITS
#   SLAI_SMOKE_RUN_ID
#   SLAI_SMOKE_TOPUP_CREDITS
#   SLAI_SMOKE_TOPUP_UNITS (raw stored units override)
#   SLAI_SMOKE_TOPUP_MINOR
#   SLAI_SMOKE_EXHAUST
#
# The script does not print passwords or management tokens.
# It prints the generated raw API key once because that key is needed to call
# OmniRoute during local smoke testing. Error output is redacted.

SCRIPT_NAME="$(basename "$0")"
WORK_DIR=""
ADMIN_COOKIES=""
USER_COOKIES=""
HTTP_STATUS=""
HTTP_BODY=""
RAW_API_KEY=""
SLAI_API_KEY_ID=""
SLAI_USER_ID=""
SLAI_PACKAGE_ID=""
SLAI_SMOKE_RUN_ID=""
PASS_MESSAGES=()
WARN_MESSAGES=()
CREDIT_UNIT_SCALE=1000000

log() {
  printf '[%s] %s\n' "$SCRIPT_NAME" "$*"
}

fail() {
  printf '[%s] ERROR: %s\n' "$SCRIPT_NAME" "$*" >&2
  print_summary 'FAILED' >&2
  exit 1
}

record_pass() {
  PASS_MESSAGES+=("$1")
  log "PASS: $1"
}

record_warn() {
  WARN_MESSAGES+=("$1")
  log "WARN: $1"
}

print_summary() {
  local status="${1:-PASSED}"
  local stream="/dev/stdout"

  if [[ "$status" != 'PASSED' ]]; then
    stream="/dev/stderr"
  fi

  {
    printf '[%s] ===== smoke test %s =====\n' "$SCRIPT_NAME" "$status"
    printf '[%s] passes: %d\n' "$SCRIPT_NAME" "${#PASS_MESSAGES[@]}"
    for item in "${PASS_MESSAGES[@]}"; do
      printf '[%s]   - %s\n' "$SCRIPT_NAME" "$item"
    done
    if ((${#WARN_MESSAGES[@]} > 0)); then
      printf '[%s] warnings: %d\n' "$SCRIPT_NAME" "${#WARN_MESSAGES[@]}"
      for item in "${WARN_MESSAGES[@]}"; do
        printf '[%s]   - %s\n' "$SCRIPT_NAME" "$item"
      done
    fi
  } >"$stream"
}

load_local_api_env() {
  if [[ "${SLAI_SMOKE_LOAD_API_ENV:-true}" != 'true' ]]; then
    return
  fi

  if [[ ! -f services/api/.env ]]; then
    return
  fi

  set -a
  # shellcheck disable=SC1091
  . services/api/.env
  set +a

  SLAI_ADMIN_EMAIL="${SLAI_ADMIN_EMAIL:-${ADMIN_SEED_EMAIL:-}}"
  SLAI_ADMIN_PASSWORD="${SLAI_ADMIN_PASSWORD:-${ADMIN_SEED_PASSWORD:-}}"
}

require_env() {
  local name="$1"

  if [[ -z "${!name:-}" ]]; then
    fail "missing required environment variable: $name"
  fi
}

require_command() {
  local name="$1"

  if ! command -v "$name" >/dev/null 2>&1; then
    fail "required command not found: $name"
  fi
}

cleanup() {
  if [[ -n "$WORK_DIR" && -d "$WORK_DIR" ]]; then
    rm -rf "$WORK_DIR"
  fi
}

redact_text() {
  local text="$1"

  if [[ -n "${RAW_API_KEY:-}" ]]; then
    text="${text//$RAW_API_KEY/[redacted-api-key]}"
  fi

  if [[ -n "${SLAI_ADMIN_PASSWORD:-}" ]]; then
    text="${text//$SLAI_ADMIN_PASSWORD/[redacted-admin-password]}"
  fi

  if [[ -n "${SLAI_USER_PASSWORD:-}" ]]; then
    text="${text//$SLAI_USER_PASSWORD/[redacted-user-password]}"
  fi

  if [[ -n "${OMNIROUTE_MANAGEMENT_TOKEN:-}" ]]; then
    text="${text//$OMNIROUTE_MANAGEMENT_TOKEN/[redacted-management-token]}"
  fi

  printf '%s' "$text" \
    | sed -E 's/sk_slai[_A-Za-z0-9.-]{12,}/[redacted-api-key]/g' \
    | sed -E 's/slai[_A-Za-z0-9.-]{12,}/[redacted-api-key]/g' \
    | sed -E 's/(Authorization: Bearer )[[:graph:]]+/\1[redacted]/g'
}

print_body_redacted() {
  local body="$1"
  local redacted

  redacted="$(redact_text "$body")"

  if jq -e . >/dev/null 2>&1 <<<"$redacted"; then
    jq . <<<"$redacted"
    return
  fi

  printf '%s\n' "$redacted"
}

make_json_login() {
  local email="$1"
  local password="$2"

  jq -n \
    --arg email "$email" \
    --arg password "$password" \
    '{email: $email, password: $password}'
}

stored_credit_units() {
  local credits="$1"
  jq -rn --arg credits "$credits" --argjson scale "$CREDIT_UNIT_SCALE" '($credits | tonumber) * $scale | round'
}

format_stored_credits() {
  local units="$1"
  jq -rn --argjson units "$units" --argjson scale "$CREDIT_UNIT_SCALE" '($units / $scale) | tostring'
}

make_json_package() {
  local package_credits
  package_credits="$(stored_credit_units "${SLAI_SMOKE_PACKAGE_CREDITS:-1000}")"

  jq -n \
    --arg name "${SLAI_SMOKE_PACKAGE_NAME:-Smoke Test Pack}" \
    --argjson creditUnits "$package_credits" \
    '{
      name: $name,
      description: "Temporary package for E2E testing",
      creditUnits: $creditUnits,
      bonusCreditUnits: 0,
      priceMinor: 1000,
      currency: "USD",
      active: true,
      sortOrder: 10
    }'
}

make_json_topup() {
  local user_id="$1"
  local package_id="$2"
  local amount_minor="$3"
  local credit_units="$4"

  jq -n \
    --arg userId "$user_id" \
    --arg packageId "$package_id" \
    --argjson amountMinor "$amount_minor" \
    --argjson creditUnits "$credit_units" \
    '{
      userId: $userId,
      packageId: $packageId,
      amountMinor: $amountMinor,
      currency: "USD",
      creditUnits: $creditUnits,
      note: "E2E smoke test top-up"
    }'
}

make_json_key_create() {
  jq -n '{name: "Smoke Test Key"}'
}

make_json_adjustment() {
  local user_id="$1"
  local delta_units="$2"
  local reason="$3"

  jq -n \
    --arg userId "$user_id" \
    --argjson deltaUnits "$delta_units" \
    --arg reason "$reason" \
    '{userId: $userId, deltaUnits: $deltaUnits, reason: $reason}'
}

make_json_chat() {
  local message="$1"

  jq -n \
    --arg model "${OMNIROUTE_SMOKE_MODEL:-gpt-4o-mini}" \
    --arg content "$message" \
    '{
      model: $model,
      messages: [
        {
          role: "user",
          content: $content
        }
      ]
    }'
}

slai_json() {
  local method="$1"
  local path="$2"
  local cookie_jar="$3"
  local body="$4"
  shift 4

  local response_file="$WORK_DIR/slai-response.json"
  local error_file="$WORK_DIR/slai-curl.err"
  local url="${SLAI_API_URL}${path}"
  local args=(-sS -X "$method" -o "$response_file" -w '%{http_code}')

  args+=(-H 'Content-Type: application/json')

  if [[ -n "$cookie_jar" ]]; then
    args+=(-b "$cookie_jar" -c "$cookie_jar")
  fi

  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi

  while (($# > 0)); do
    args+=(-H "$1")
    shift
  done

  args+=("$url")

  if ! HTTP_STATUS="$(curl "${args[@]}" 2>"$error_file")"; then
    local curl_error
    curl_error="$(cat "$error_file")"
    fail "curl failed for $method $path: $(redact_text "$curl_error")"
  fi

  HTTP_BODY="$(cat "$response_file")"
}

omniroute_json() {
  local method="$1"
  local path="$2"
  local body="$3"
  shift 3

  local response_file="$WORK_DIR/omniroute-response.json"
  local error_file="$WORK_DIR/omniroute-curl.err"
  local url="${OMNIROUTE_BASE_URL}${path}"
  local args=(-sS -X "$method" -o "$response_file" -w '%{http_code}')

  args+=(-H 'Content-Type: application/json')

  while (($# > 0)); do
    args+=(-H "$1")
    shift
  done

  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi

  args+=("$url")

  if ! HTTP_STATUS="$(curl "${args[@]}" 2>"$error_file")"; then
    local curl_error
    curl_error="$(cat "$error_file")"
    fail "curl failed for OmniRoute $method $path: $(redact_text "$curl_error")"
  fi

  HTTP_BODY="$(cat "$response_file")"
}

expect_2xx() {
  local label="$1"

  if [[ "$HTTP_STATUS" != 2* ]]; then
    printf '[%s] %s returned HTTP %s\n' "$SCRIPT_NAME" "$label" "$HTTP_STATUS" >&2
    print_body_redacted "$HTTP_BODY" >&2
    if [[ "$HTTP_STATUS" == '501' && "$HTTP_BODY" == *'omniroute_sync_not_implemented'* ]]; then
      printf '[%s] Hint: the SLAI API is using the local OmniRoute stub. Set OMNIROUTE_ENABLED=true, load services/api/.env, and restart the API process.\n' "$SCRIPT_NAME" >&2
    fi
    print_summary 'FAILED' >&2
    exit 1
  fi
}

expect_status_or_continue() {
  local label="$1"
  local expected_prefix="$2"

  if [[ "$HTTP_STATUS" == "$expected_prefix"* ]]; then
    return 0
  fi

  log "$label returned HTTP $HTTP_STATUS"
  print_body_redacted "$HTTP_BODY"
  return 1
}

json_value() {
  local query="$1"
  local body="$2"

  jq -r "$query" <<<"$body"
}

print_step_result() {
  local label="$1"

  log "$label"
  print_body_redacted "$HTTP_BODY"
}

check_health() {
  log "checking SLAI health"

  local response
  response="$(curl -sS "${SLAI_API_URL}/healthz")"

  if [[ "$response" != $'OK\n' && "$response" != 'OK' ]]; then
    fail "unexpected /healthz response: $(redact_text "$response")"
  fi

  record_pass "SLAI health endpoint responded"
}

check_readiness() {
  log "checking SLAI readiness"

  slai_json GET /readyz '' ''
  expect_2xx 'SLAI readiness'
  print_step_result 'SLAI ready'
  record_pass "SLAI readiness endpoint responded"
}

login_admin() {
  log "logging in as admin"

  local body
  body="$(make_json_login "$SLAI_ADMIN_EMAIL" "$SLAI_ADMIN_PASSWORD")"

  slai_json POST /v1/auth/login "$ADMIN_COOKIES" "$body"
  expect_2xx 'admin login'
  print_step_result 'admin login succeeded'
  record_pass "admin login succeeded"
}

assert_sync_configuration() {
  log "checking SLAI OmniRoute sync configuration"

  slai_json GET /v1/admin/usage/sync-status "$ADMIN_COOKIES" ''
  expect_2xx 'sync status'

  local omniroute_enabled
  local sync_mode
  local worker_enabled
  local last_error

  omniroute_enabled="$(json_value '.sync_status.omniroute_enabled' "$HTTP_BODY")"
  sync_mode="$(json_value '.sync_status.sync_mode' "$HTTP_BODY")"
  worker_enabled="$(json_value '.sync_status.worker_enabled' "$HTTP_BODY")"
  last_error="$(json_value '.sync_status.last_error // ""' "$HTTP_BODY")"

  if [[ "$omniroute_enabled" != 'true' ]]; then
    fail 'SLAI reports omniroute_enabled=false. The smoke test requires the real OmniRoute HTTP client; restart the API with OMNIROUTE_ENABLED=true.'
  fi

  if [[ "$sync_mode" != 'call_logs' ]]; then
    fail "SLAI reports sync_mode=$sync_mode. The E2E smoke test requires OMNIROUTE_USAGE_SYNC_MODE=call_logs."
  fi

  if [[ "$worker_enabled" != 'true' ]]; then
    record_warn 'automatic usage sync worker is disabled; manual sync will still be used by the smoke test'
  fi

  if [[ -n "$last_error" && "$last_error" != 'null' ]]; then
    record_warn "previous sync error is still recorded: $(redact_text "$last_error")"
  fi

  print_step_result 'usage sync status before test'
  record_pass "SLAI is using OmniRoute call-log sync"
}

signup_or_login_user() {
  log "creating normal user, or logging in if it already exists"

  local body
  body="$(make_json_login "$SLAI_USER_EMAIL" "$SLAI_USER_PASSWORD")"

  slai_json POST /v1/auth/signup "$USER_COOKIES" "$body"

  if [[ "$HTTP_STATUS" == 2* ]]; then
    print_step_result 'user signup succeeded'
  else
    log "signup did not succeed; trying login instead"
    slai_json POST /v1/auth/login "$USER_COOKIES" "$body"
    expect_2xx 'user login'
    print_step_result 'user login succeeded'
  fi

  slai_json GET /v1/me "$USER_COOKIES" ''
  expect_2xx 'user /v1/me'

  SLAI_USER_ID="$(json_value '.user.id' "$HTTP_BODY")"

  if [[ -z "$SLAI_USER_ID" || "$SLAI_USER_ID" == 'null' ]]; then
    fail 'could not determine SLAI user ID'
  fi

  log "SLAI user ID: $SLAI_USER_ID"
  record_pass "test user session ready"
}

ensure_package() {
  log "finding or creating smoke-test credit package"

  local package_name="${SLAI_SMOKE_PACKAGE_NAME:-Smoke Test Pack}"
  local found_id
  local found_active

  slai_json GET /v1/admin/packages "$ADMIN_COOKIES" ''
  expect_2xx 'list admin packages'

  found_id="$(jq -r --arg name "$package_name" '[.packages[]? | select(.name == $name)] | sort_by(.createdAt) | last | .id // empty' <<<"$HTTP_BODY")"
  found_active="$(jq -r --arg name "$package_name" '[.packages[]? | select(.name == $name)] | sort_by(.createdAt) | last | .active // empty' <<<"$HTTP_BODY")"

  if [[ -n "$found_id" ]]; then
    SLAI_PACKAGE_ID="$found_id"
    log "using existing package: $SLAI_PACKAGE_ID"

    if [[ "$found_active" != 'true' ]]; then
      log "existing smoke package is inactive; activating it"
      slai_json PATCH "/v1/admin/packages/${SLAI_PACKAGE_ID}" "$ADMIN_COOKIES" '{"active":true}'
      expect_2xx 'activate existing package'
    fi

    record_pass "smoke credit package ready"
    return
  fi

  log "creating smoke-test credit package"

  local body
  body="$(make_json_package)"

  slai_json POST /v1/admin/packages "$ADMIN_COOKIES" "$body"
  expect_2xx 'create package'

  SLAI_PACKAGE_ID="$(json_value '.package.id' "$HTTP_BODY")"

  if [[ -z "$SLAI_PACKAGE_ID" || "$SLAI_PACKAGE_ID" == 'null' ]]; then
    fail 'could not determine package ID'
  fi

  print_step_result 'package created'
  log "package ID: $SLAI_PACKAGE_ID"
  record_pass "smoke credit package created"
}

confirm_public_package() {
  log "confirming smoke package appears in public packages API"

  slai_json GET /v1/packages '' ''
  expect_2xx 'public packages list'

  local public_count
  public_count="$(jq -r --arg id "$SLAI_PACKAGE_ID" '[.packages[]? | select(.id == $id and .active == true)] | length' <<<"$HTTP_BODY")"

  if [[ "$public_count" == '0' ]]; then
    fail "smoke package $SLAI_PACKAGE_ID was not visible in /v1/packages"
  fi

  print_step_result 'public packages list'
  record_pass "smoke credit package visible publicly"
}

top_up_user() {
  log "manual top-up for smoke-test user"

  local units
  local amount="${SLAI_SMOKE_TOPUP_MINOR:-1000}"
  if [[ -n "${SLAI_SMOKE_TOPUP_UNITS:-}" ]]; then
    units="$SLAI_SMOKE_TOPUP_UNITS"
    record_warn 'SLAI_SMOKE_TOPUP_UNITS is raw stored units; prefer SLAI_SMOKE_TOPUP_CREDITS for displayed credits'
  else
    units="$(stored_credit_units "${SLAI_SMOKE_TOPUP_CREDITS:-1000}")"
  fi
  local idempotency_key="smoke-topup-${SLAI_SMOKE_RUN_ID}"
  local body

  body="$(make_json_topup "$SLAI_USER_ID" "$SLAI_PACKAGE_ID" "$amount" "$units")"

  slai_json \
    POST \
    /v1/admin/payments/manual-topup \
    "$ADMIN_COOKIES" \
    "$body" \
    "Idempotency-Key: $idempotency_key"

  expect_2xx 'manual top-up'
  print_step_result 'manual top-up succeeded'
  record_pass "manual top-up completed"
}

create_or_rotate_api_key() {
  log "creating user API key"

  local body
  body="$(make_json_key_create)"

  slai_json POST /v1/api-key "$USER_COOKIES" "$body"

  if [[ "$HTTP_STATUS" == 409 ]]; then
    log "active key already exists; rotating to obtain a one-time raw key"
    slai_json POST /v1/api-key/rotate "$USER_COOKIES" '{}'
  fi

  expect_2xx 'create or rotate API key'

  RAW_API_KEY="$(json_value '.raw_api_key' "$HTTP_BODY")"
  SLAI_API_KEY_ID="$(json_value '.api_key.id' "$HTTP_BODY")"

  if [[ -z "$RAW_API_KEY" || "$RAW_API_KEY" == 'null' ]]; then
    fail 'API create/rotate response did not include raw_api_key'
  fi

  if [[ -z "$SLAI_API_KEY_ID" || "$SLAI_API_KEY_ID" == 'null' ]]; then
    fail 'API create/rotate response did not include api_key.id'
  fi

  log 'API key create/rotate response, with raw key redacted:'
  print_body_redacted "$HTTP_BODY"

  printf '[%s] Generated raw API key, shown once for local testing: %s\n' \
    "$SCRIPT_NAME" \
    "$RAW_API_KEY"

  log "SLAI API key ID: $SLAI_API_KEY_ID"
  record_pass "SLAI-created raw API key available for OmniRoute call"
}

confirm_key_metadata() {
  log "confirming key metadata from user endpoint"

  slai_json GET /v1/api-key "$USER_COOKIES" ''
  expect_2xx 'user API key metadata'
  print_step_result 'user API key metadata'

  log "confirming key metadata from admin endpoint"

  slai_json GET "/v1/admin/users/${SLAI_USER_ID}/api-key" "$ADMIN_COOKIES" ''
  expect_2xx 'admin API key metadata'
  print_step_result 'admin API key metadata'
  record_pass "API key metadata visible without raw key leakage"
}

optionally_list_omniroute_keys() {
  if [[ -z "${OMNIROUTE_MANAGEMENT_TOKEN:-}" ]]; then
    log 'OMNIROUTE_MANAGEMENT_TOKEN is not set; skipping direct OmniRoute key list'
    return
  fi

  log "listing OmniRoute keys through management API"

  omniroute_json \
    GET \
    /api/keys \
    '' \
    "Authorization: Bearer ${OMNIROUTE_MANAGEMENT_TOKEN}"

  expect_2xx 'OmniRoute key list'
  print_step_result 'OmniRoute key list, secrets redacted'
  record_pass "OmniRoute management API listed keys"
}

call_omniroute_chat() {
  local message="$1"
  local label="$2"
  local body

  body="$(make_json_chat "$message")"

  log "$label"

  omniroute_json \
    POST \
    /v1/chat/completions \
    "$body" \
    "Authorization: Bearer ${RAW_API_KEY}"

  expect_2xx 'OmniRoute chat completion'
  print_step_result 'OmniRoute chat response, secrets redacted'
  record_pass "$label"
}

sync_usage() {
  local label="$1"

  log "$label"

  slai_json POST /v1/admin/usage/sync "$ADMIN_COOKIES" '{}'
  expect_2xx 'manual usage sync'
  print_step_result 'manual usage sync result'
  record_pass "$label"
}

show_usage_balance_and_ledger() {
  log "listing user usage events"

  slai_json GET '/v1/usage?limit=20' "$USER_COOKIES" ''
  expect_2xx 'user usage list'
  print_step_result 'user usage list'

  local usage_count
  usage_count="$(json_value '[.usage[]?] | length' "$HTTP_BODY")"
  if [[ "$usage_count" == '0' ]]; then
    fail 'no user usage events were visible after sync'
  fi

  log "listing admin usage events for smoke-test user"

  slai_json GET "/v1/admin/usage?user_id=${SLAI_USER_ID}&limit=20" "$ADMIN_COOKIES" ''
  expect_2xx 'admin usage list'
  print_step_result 'admin usage list'

  local admin_usage_count
  admin_usage_count="$(json_value '[.usage[]?] | length' "$HTTP_BODY")"
  if [[ "$admin_usage_count" == '0' ]]; then
    fail 'no admin usage events were visible after sync'
  fi

  log "showing user balance"

  slai_json GET /v1/balance "$USER_COOKIES" ''
  expect_2xx 'user balance'
  print_step_result 'user balance'

  local available
  available="$(json_value '.balance.availableUnits' "$HTTP_BODY")"

  log "showing user ledger"

  slai_json GET '/v1/ledger?limit=20' "$USER_COOKIES" ''
  expect_2xx 'user ledger'
  print_step_result 'user ledger'

  local debit_count
  debit_count="$(json_value '[.ledger[]? | select(.type == "usage_debit")] | length' "$HTTP_BODY")"
  if [[ "$debit_count" == '0' ]]; then
    fail 'no usage_debit ledger entry was visible after sync'
  fi

  log "listing user payment history"

  slai_json GET '/v1/payments?limit=20' "$USER_COOKIES" ''
  expect_2xx 'user payments list'
  print_step_result 'user payments list'

  local available_credits
  available_credits="$(format_stored_credits "$available")"
  record_pass "usage, balance, ledger, and payments visible after sync; available balance: $available_credits credits ($available stored units)"
}

get_available_balance() {
  slai_json GET /v1/balance "$USER_COOKIES" ''
  expect_2xx 'user balance'
  json_value '.balance.availableUnits' "$HTTP_BODY"
}

check_duplicate_sync() {
  log "capturing balance before duplicate sync"

  local before
  local after
  before="$(get_available_balance)"

  sync_usage 'running duplicate sync check'

  after="$(get_available_balance)"

  log "balance before duplicate sync: $before"
  log "balance after duplicate sync:  $after"

  if [[ "$before" != "$after" ]]; then
    record_warn 'balance changed during duplicate sync; check whether worker or new logs ran concurrently'
  else
    record_pass 'duplicate sync did not change balance'
  fi
}

maybe_exhaust_balance() {
  if [[ "${SLAI_SMOKE_EXHAUST:-false}" != 'true' ]]; then
    log 'SLAI_SMOKE_EXHAUST is not true; skipping balance-exhaustion suspension test'
    return
  fi

  log 'running optional balance-exhaustion suspension test'

  local available
  local delta
  local body
  local idempotency_key

  available="$(get_available_balance)"

  if [[ "$available" =~ ^-?[0-9]+$ && "$available" -gt 1 ]]; then
    delta=$((1 - available))
    idempotency_key="smoke-adjust-down-${SLAI_SMOKE_RUN_ID}"
    body="$(make_json_adjustment \
      "$SLAI_USER_ID" \
      "$delta" \
      'Prepare smoke test key suspension')"

    slai_json \
      POST \
      /v1/admin/ledger/adjustments \
      "$ADMIN_COOKIES" \
      "$body" \
      "Idempotency-Key: $idempotency_key"

    expect_2xx 'admin adjustment before exhaustion'
    print_step_result 'admin adjustment before exhaustion succeeded'
  fi

  call_omniroute_chat \
    'Reply with a short sentence for the SLAI suspension smoke test.' \
    'calling OmniRoute to exhaust remaining balance'

  sync_usage 'syncing usage expected to suspend key'

  slai_json GET "/v1/admin/users/${SLAI_USER_ID}/api-key" "$ADMIN_COOKIES" ''
  expect_2xx 'admin key metadata after exhaustion'
  print_step_result 'admin key metadata after exhaustion'

  local status
  status="$(json_value '.api_key.status' "$HTTP_BODY")"

  if [[ "$status" != 'SUSPENDED' ]]; then
    fail "expected API key status SUSPENDED after exhaustion, got: $status"
  fi

  log 'key suspension confirmed'
  record_pass 'key suspension confirmed after balance exhaustion'
}

show_sync_status() {
  log "checking usage sync status"

  slai_json GET /v1/admin/usage/sync-status "$ADMIN_COOKIES" ''
  expect_2xx 'sync status'
  print_step_result 'usage sync status'
  record_pass "sync status endpoint available after test"
}

main() {
  load_local_api_env

  SLAI_API_URL="${SLAI_API_URL:-http://localhost:8080}"

  require_env SLAI_ADMIN_EMAIL
  require_env SLAI_ADMIN_PASSWORD
  require_env SLAI_USER_EMAIL
  require_env SLAI_USER_PASSWORD
  require_env OMNIROUTE_BASE_URL

  require_command curl
  require_command jq
  require_command sed

  SLAI_API_URL="${SLAI_API_URL%/}"
  OMNIROUTE_BASE_URL="${OMNIROUTE_BASE_URL%/}"
  OMNIROUTE_SMOKE_MODEL="${OMNIROUTE_SMOKE_MODEL:-gpt-4o-mini}"
  SLAI_SMOKE_RUN_ID="${SLAI_SMOKE_RUN_ID:-$(date +%Y%m%d%H%M%S)}"

  WORK_DIR="$(mktemp -d -t slai-smoke.XXXXXX)"
  ADMIN_COOKIES="$WORK_DIR/admin.cookies"
  USER_COOKIES="$WORK_DIR/user.cookies"

  trap cleanup EXIT

  log "using SLAI API URL: $SLAI_API_URL"
  log "using OmniRoute URL: $OMNIROUTE_BASE_URL"
  log "using OmniRoute model: $OMNIROUTE_SMOKE_MODEL"
  log "using smoke run ID: $SLAI_SMOKE_RUN_ID"
  log 'passwords and management tokens will not be printed'

  check_health
  check_readiness
  login_admin
  assert_sync_configuration
  signup_or_login_user
  ensure_package
  confirm_public_package
  top_up_user
  create_or_rotate_api_key
  confirm_key_metadata
  optionally_list_omniroute_keys

  call_omniroute_chat \
    'Reply with the word SLAI.' \
    'calling OmniRoute with the SLAI-created key'

  sync_usage 'triggering manual usage sync'
  show_usage_balance_and_ledger
  check_duplicate_sync
  maybe_exhaust_balance
  show_sync_status

  print_summary 'PASSED'
}

main "$@"

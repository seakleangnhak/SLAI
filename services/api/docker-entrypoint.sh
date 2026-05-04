#!/bin/sh
set -eu

if [ "${1:-}" = "/app/slai-api" ]; then
  shift
fi

if [ "$#" -eq 0 ]; then
  set -- serve
fi

run_migrations() {
  attempts="${SLAI_STARTUP_DB_RETRIES:-30}"
  delay="${SLAI_STARTUP_DB_RETRY_DELAY_SECONDS:-2}"
  attempt=1

  while true; do
    if /app/slai-api migrate up; then
      return 0
    fi

    if [ "$attempt" -ge "$attempts" ]; then
      echo "[slai-api] database migrations failed after ${attempts} attempts" >&2
      return 1
    fi

    echo "[slai-api] database migration attempt ${attempt}/${attempts} failed; retrying in ${delay}s" >&2
    attempt=$((attempt + 1))
    sleep "$delay"
  done
}

case "${1:-}" in
  serve|server)
    if [ "${SLAI_AUTO_MIGRATE:-true}" = "true" ]; then
      echo "[slai-api] running database migrations"
      run_migrations
    else
      echo "[slai-api] skipping database migrations because SLAI_AUTO_MIGRATE=false"
    fi

    if [ "${SLAI_AUTO_SEED_ADMIN:-true}" = "true" ]; then
      if [ -n "${ADMIN_SEED_EMAIL:-}" ] && [ -n "${ADMIN_SEED_PASSWORD:-}" ]; then
        echo "[slai-api] ensuring admin seed user exists"
        /app/slai-api seed-admin
      else
        echo "[slai-api] skipping admin seed because ADMIN_SEED_EMAIL or ADMIN_SEED_PASSWORD is empty"
      fi
    else
      echo "[slai-api] skipping admin seed because SLAI_AUTO_SEED_ADMIN=false"
    fi
    ;;
esac

exec /app/slai-api "$@"

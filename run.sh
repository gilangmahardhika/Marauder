#!/bin/sh
# Start the API and the SvelteKit client. Ctrl-C stops both.
# Usage: ./run.sh [target-repo]
# The data directory stays outside the target repo.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
repo=${1:-${ORCHESTRATOR_REPO:-$root}}
addr=${ORCHESTRATOR_ADDR:-127.0.0.1:8787}
ui=${ORCHESTRATOR_UI:-http://127.0.0.1:4173}

if ! command -v go >/dev/null 2>&1; then
  echo "Go is not on PATH." >&2
  exit 1
fi
if ! command -v npm >/dev/null 2>&1; then
  echo "npm is not on PATH." >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is not on PATH." >&2
  exit 1
fi
if [ ! -f "$root/backend/.env" ] && [ -z "${ORCHESTRATOR_DATABASE_URL:-}" ]; then
  echo "Create backend/.env with ORCHESTRATOR_DATABASE_URL. See README.md." >&2
  exit 1
fi

api_pid=
ui_pid=
cleanup() {
  if [ -n "$ui_pid" ]; then
    pkill -P "$ui_pid" 2>/dev/null || true
    kill "$ui_pid" 2>/dev/null || true
  fi
  if [ -n "$api_pid" ]; then
    pkill -P "$api_pid" 2>/dev/null || true
    kill "$api_pid" 2>/dev/null || true
  fi
}
trap cleanup INT TERM EXIT

(
  cd "$root/backend"
  if [ -n "${ORCHESTRATOR_DATA:-}" ]; then
    mkdir -p "$ORCHESTRATOR_DATA"
    chmod 700 "$ORCHESTRATOR_DATA"
    exec go run ./cmd/orchestrator serve --repo "$repo" --addr "$addr" --data "$ORCHESTRATOR_DATA"
  fi
  exec go run ./cmd/orchestrator serve --repo "$repo" --addr "$addr"
) &
api_pid=$!

echo "Waiting for the API on http://$addr"
i=0
while [ "$i" -lt 90 ]; do
  if ! kill -0 "$api_pid" 2>/dev/null; then
    echo "The API exited before it listened." >&2
    exit 1
  fi
  code=$(curl -s -o /dev/null -w "%{http_code}" --max-time 1 "http://$addr/api/cards" || true)
  case "$code" in
    200|401|403|429) break ;;
  esac
  i=$((i + 1))
  sleep 1
done
if [ "$i" -eq 90 ]; then
  echo "The API did not listen on http://$addr." >&2
  exit 1
fi

(
  cd "$root/frontend"
  if [ ! -d node_modules ]; then
    npm ci
  fi
  exec npm run dev
) &
ui_pid=$!

echo "API  http://$addr"
echo "UI   $ui"
echo "Paste the UI token from the API log into the login page. Ctrl-C stops both."

while kill -0 "$api_pid" 2>/dev/null && kill -0 "$ui_pid" 2>/dev/null; do
  sleep 1
done

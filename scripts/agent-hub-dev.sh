#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$REPO_ROOT/web/agent-hub"
HUB_HOST="${HUB_HOST:-127.0.0.1}"
HUB_PORT="${HUB_PORT:-46001}"
WEB_PORT="${WEB_PORT:-5173}"
HUB_DB="${HUB_DB:-$HOME/.agent-toolkit/hub.db}"
HUB_BIN="/tmp/agent-hub-dev-bin"

if ! command -v bun >/dev/null 2>&1; then
  echo "bun is required but not found in PATH"
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  echo "go is required but not found in PATH"
  exit 1
fi

cd "$WEB_DIR"
bun install >/dev/null
bun run build >/dev/null

cleanup() {
  if [[ -n "${WEB_PID:-}" ]]; then
    kill "$WEB_PID" >/dev/null 2>&1 || true
    wait "$WEB_PID" 2>/dev/null || true
  fi
  if [[ -n "${HUB_PID:-}" ]]; then
    kill "$HUB_PID" >/dev/null 2>&1 || true
    wait "$HUB_PID" 2>/dev/null || true
  fi
  rm -f "$HUB_BIN" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

cd "$REPO_ROOT"
go build -o "$HUB_BIN" ./cmd/agent-hub
"$HUB_BIN" \
  --listen "$HUB_HOST:$HUB_PORT" \
  --db "$HUB_DB" \
  --web-dir "$WEB_DIR/dist" >/tmp/agent-hub-dev.log 2>&1 &
HUB_PID=$!

ready=false
for _ in $(seq 1 80); do
  if curl -fsS "http://$HUB_HOST:$HUB_PORT/healthz" >/dev/null 2>&1; then
    ready=true
    break
  fi
  sleep 0.25
done

if [[ "$ready" != true ]]; then
  echo "agent-hub backend failed to start. See /tmp/agent-hub-dev.log"
  exit 1
fi

echo "agent-hub backend: http://$HUB_HOST:$HUB_PORT"
echo "agent-hub web dev: http://$HUB_HOST:$WEB_PORT"
echo "Press Ctrl+C to stop both processes."

cd "$WEB_DIR"
bun run dev:web --host "$HUB_HOST" --port "$WEB_PORT" &
WEB_PID=$!
wait "$WEB_PID"

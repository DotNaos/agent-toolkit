#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RAW_SCRIPT="$REPO_ROOT/scripts/agent-hub-dev-raw.sh"

if [[ "${AGENT_HUB_DEV_ALLOW_DIRECT:-}" == "1" ]]; then
  exec "$RAW_SCRIPT"
fi

if ! command -v portless >/dev/null 2>&1; then
  cat <<'EOF'
agent-hub dev must be started through portless.

Install portless first:
  npm install -g portless

If you intentionally want to bypass portless for this run, re-run with:
  AGENT_HUB_DEV_ALLOW_DIRECT=1 ./scripts/agent-hub-dev.sh
EOF
  exit 1
fi

exec env AGENT_HUB_DEV_VIA_PORTLESS=1 portless run --name agent-hub bash "$RAW_SCRIPT"

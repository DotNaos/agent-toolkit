# agent-memory

Use this skill for local preference memory + proxy hint injection for coding agents.

## Build

```bash
go build -o bin/agent-memory ./cmd/agent-memory
```

## Core workflow (MVP)

Start daemon/API:

```bash
agent-memory daemon
```

Seed defaults (`bun`, `uv`):

```bash
agent-memory memory seed
```

Add a memory manually:

```bash
agent-memory memory add \
  --scope global \
  --category tooling \
  --title "Frontend tooling" \
  --content "Use bun by default for frontend work unless repo conventions override it." \
  --tags bun,frontend
```

Sync repo-specific rules from `AGENTS.md`, `SKILL.md`, `README.md`:

```bash
agent-memory repo sync --repo-path /path/to/repo
```

Search memory:

```bash
agent-memory memory search --query "frontend package manager" --repo-path /path/to/repo
```

## mitmproxy addon (best effort)

```bash
AGENT_MEMORY_API_URL=http://127.0.0.1:45229 \
AGENT_MEMORY_PROXY_API_VERSION=v2 \
AGENT_MEMORY_REPO_PATH=/path/to/your/project \
mitmdump -s tools/agent-memory/mitm_addon.py
```

The addon is fail-open: if a request cannot be transformed (pinning, compression, unknown shape), it passes through unchanged and logs a compatibility event.

## v2 (`jj`-backed) workflow (Phase 1)

Start a task workspace and create an episode:

```bash
curl -s -X POST http://127.0.0.1:45229/v2/memory/task/start \
  -H 'Content-Type: application/json' \
  --data '{"repo_path":"/path/to/project"}'

curl -s -X POST http://127.0.0.1:45229/v2/memory/episode/create \
  -H 'Content-Type: application/json' \
  --data '{"repo_path":"/path/to/project","task_id":"<task-id>","targets":["topic/tooling"],"source":"manual","kind":"manual-note","step_summary":"Document preference","decisions":["Use bun by default for frontend work."]}'
```

Consolidate snapshots and resolve them for proxy injection:

```bash
curl -s -X POST http://127.0.0.1:45229/v2/memory/task/end \
  -H 'Content-Type: application/json' \
  --data '{"repo_path":"/path/to/project","task_id":"<task-id>"}'

curl -s -X POST http://127.0.0.1:45229/v2/memory/snapshot/resolve \
  -H 'Content-Type: application/json' \
  --data '{"repo_path":"/path/to/project","targets":["topic/tooling"]}'
```

# agent-toolkit

`agent-toolkit` is the repository name (monorepo), not a CLI command name.

## Project rule

- Every CLI tool in this repository must be its own project.
- Each tool must have its own binary name.
- Do not use `agent-toolkit` as the runtime CLI command name.

## skills.sh compatibility

Every tool in this monorepo must ship a matching agent skill.

- Skill files live under `skills/<tool-name>/SKILL.md`.
- Current skills:
    - `skills/ui-loop/SKILL.md`
    - `skills/ui-clarity-loop/SKILL.md`
    - `skills/requirements-ui-redesign/SKILL.md`
    - `skills/agent-toolkit-skill/SKILL.md`
    - `skills/agent-chat/SKILL.md`
    - `skills/agent-delegate/SKILL.md`
    - `skills/agent-memory/SKILL.md`
    - `skills/agent-hub/SKILL.md`
    - `skills/repo-branch-protection/SKILL.md`
    - `skills/readme-tutorial-flow/SKILL.md`
    - `skills/evidence-driven-validation/SKILL.md`

Install all skills from this repo (global, non-interactive):

```bash
npx skills add DotNaos/agent-toolkit/skills --all -g
```

Install one skill only:

```bash
npx skills add DotNaos/agent-toolkit/skills --skill ui-loop -g -y
```

Preview what is available before installing:

```bash
npx skills add DotNaos/agent-toolkit/skills --list
```

Reference:

- https://skills.sh/
- `npx skills --help`

## Shell completions

This repo now ships zsh completion support for its main CLIs:

- `agent-chat`
- `agent-memory`
- `ui-loop`
- `agent-hub`
- `agent-delegate`

Generate and install the completion files into `~/.zsh/completions`:

```bash
./scripts/install-zsh-completions.sh
```

The installed completions are bound both to the command names and to the local repo binaries under `./bin/...`.

## Branch protection setup skill

This repository also ships a reusable skill to standardize branch protection and PR-first workflows across repositories:

- `skills/repo-branch-protection/SKILL.md`

Install just this one:

```bash
npx skills add DotNaos/agent-toolkit/skills --skill repo-branch-protection -g -y
```

## Current milestone tool

The current Milestone 1/1.1 UI automation tool can be built with any binary name.

Example:

```bash
go build -o bin/ui-loop ./cmd/ui-loop
./bin/ui-loop map-ui
./bin/ui-loop click --id X002
./bin/ui-loop type --id X003 --text "Hallo Welt" --submit
```

Because the command name is derived from the binary filename, this works with any chosen tool name.

## Agent chat (always-on workers)

`agent-chat watch` can keep an agent inbox alive without manually running `wait` each time.

Example:

```bash
export AGENT_CHAT_SERVER_URL=http://127.0.0.1:45217
export AGENT_CHAT_DB_PATH=/tmp/agent-chat-shared.db

# Always-on consumer (acks automatically)
agent-chat watch --agent agent-a --thread collab --auto-ack

# Or run a handler command per message (ack after handler success)
agent-chat watch --agent agent-a --thread collab --handler 'cat >/tmp/last-message.json'
```

## Agent memory (local preferences + proxy hint injection)

`agent-memory` stores persistent coding preferences and repo-specific guidelines (for example `bun` for frontend and `uv` for Python), and exposes a local API that a MITM proxy addon can call to inject short hints into provider requests.

It now also includes a Phase-1 `jj`-backed v2 memory flow (`/v2/*`) with:

- immutable episode commits
- immutable consolidated snapshots (`rev-N`)
- snapshot-based proxy context injection with prompt hygiene and hard memory budgeting

Example:

```bash
go build -o bin/agent-memory ./cmd/agent-memory
./bin/agent-memory daemon
./bin/agent-memory memory seed
./bin/agent-memory repo sync --repo-path /path/to/repo
```

## Agent hub (web/pwa + human approvals)

`agent-hub` is a local web app for realtime group chat (`owner` + agents) with mandatory human-in-the-loop approvals for risky actions.
The UI stack uses `React + TailwindCSS + shadcn/ui`.

Run (one command, bun-based, enforced through portless):

```bash
./scripts/agent-hub-dev.sh
```

Then open:

- the printed `.localhost` URL from the script output

The standard dev entrypoint now requires portless and will refuse a direct start.
If you intentionally need to bypass portless for one run, use:

```bash
AGENT_HUB_DEV_ALLOW_DIRECT=1 ./scripts/agent-hub-dev.sh
```

Default behavior:

- Realtime updates via SSE (`/v1/events/stream`)
- Risky dispatches (write/edit/delete/deploy/external side effects) create blocking approvals
- Approval dialog supports enum choices, multi-choice arrays, and free text
- Delegate dispatches can target `gemini`, `claude`, `copilot`, or `codex` via `agent-delegate`

## Agent delegate (TS implementation + Go wrapper)

`agent-delegate` keeps the actual delegation runtime in TypeScript and exposes the same contract through a thin Go wrapper so the repo still ships a standard CLI entrypoint under `cmd/agent-delegate/`.

Configured adapters can expose allowlisted models and a default model. `copilot` model entries may also include a `multiplier` field so the UI and callers can show GitHub premium-request weighting next to the model choice. These lists are repo-configured allowlists, not exhaustive runtime discovery.

Build the wrapper:

```bash
go build -o bin/agent-delegate ./cmd/agent-delegate
```

Run the TypeScript implementation directly:

```bash
cd tools/agent-delegate
bun run src/index.ts list-adapters --config ../../agent-delegate.json
```

Run through the Go wrapper:

```bash
./bin/agent-delegate list-adapters --config agent-delegate.json
```

## Tool isolation policy (must follow)

This repository is a monorepo of multiple independent CLI tools.
Tools must be developed in isolation so multiple agents can work in parallel without blocking each other.

### 1) Ownership boundaries

- A tool owns its own binary entrypoint under `cmd/<tool-name>/`.
- A tool owns its own implementation packages under `internal/<tool-name>*`.
- A tool-specific helper (for example platform code) should live under a tool-specific path (for example `tools/<tool-name>/...`).
- Existing paths in this repo currently map like this:
    - UI automation tool: `cmd/ui-loop/`, `internal/uilloopcli/`, `tools/ui-loop/`
    - Agent chat tool: `cmd/agent-chat/`, `internal/chatcli/`, `internal/chatd/`
    - Agent hub tool: `cmd/agent-hub/`, `internal/hubapi/`, `internal/hubstore/`, `internal/hubworker/`, `web/agent-hub/`

### 2) No implicit cross-tool edits

- Do not edit files owned by another tool unless explicitly requested.
- Do not mix unrelated tool changes in one task/commit.
- Shared changes (dependencies, shared utils, root config) are allowed only when required by the requested task.

### 2.1) Shared package rules (allowed, but strict)

- Shared packages are allowed for stable, low-level primitives used by multiple tools (for example JSON output helpers).
- Place shared code under `internal/shared/<package>/`.
- Keep shared APIs minimal and explicit; do not create a generic \"god util\" package.
- A shared change must include verification for every tool that consumes the shared API.

### 3) Agent safety workflow

- Before editing, check `git status --short` and the target paths.
- If unexpected modifications appear in unrelated files: stop and ask before continuing.
- Scope formatting/linting/testing to affected packages when possible (avoid broad commands that touch unrelated files).

### 4) New tools

- Every new tool must have:
    - its own binary name
    - isolated package paths
    - tests in its own package tree
- Keep the public CLI contract stable per tool (JSON output shape should not accidentally change across tools).

# agent-toolkit

`agent-toolkit` is the repository name (monorepo), not a CLI command name.

## Project rule

- Every CLI tool in this repository must be its own project.
- Each tool must have its own binary name.
- Do not use `agent-toolkit` as the runtime CLI command name.

## Current milestone tool

The current Milestone 1/1.1 UI automation tool can be built with any binary name.

Example:

```bash
go build -o bin/ui-loop .
./bin/ui-loop map-ui
./bin/ui-loop click --id X002
./bin/ui-loop type --id X003 --text "Hallo Welt" --submit
```

Because the command name is derived from the binary filename, this works with any chosen tool name.

## Tool isolation policy (must follow)

This repository is a monorepo of multiple independent CLI tools.  
Tools must be developed in isolation so multiple agents can work in parallel without blocking each other.

### 1) Ownership boundaries

- A tool owns its own binary entrypoint under `cmd/<tool-name>/`.
- A tool owns its own implementation packages under `internal/<tool-name>*`.
- A tool-specific helper (for example platform code) should live under a tool-specific path (for example `tools/<tool-name>/...`).
- Existing paths in this repo currently map like this:
  - UI automation tool: `main.go`, `cmd/`, `tools/axdump/`
  - Agent chat tool: `cmd/agent-chat/`, `internal/chatcli/`, `internal/chatd/`

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

---
name: codex-handover
description: Create Codex handover briefings and kick off or resume target-project Codex sessions from another session's findings.
---

# codex-handover

Use this skill when a user wants to carry a finding, pattern, or partially explored idea from one Codex session into another project or session.

## Core idea

A handover is not a loose summary. It is an actionable briefing with:

- source session id
- source project
- target project
- implementation mode
- the idea to carry forward
- requested change
- acceptance criteria
- constraints

## Build

```bash
go build -o bin/codex-handover ./cmd/codex-handover
```

## Create a briefing

```bash
codex-handover create \
  --source-project /path/to/source \
  --target-project /path/to/target \
  --title "Project goals file" \
  --mode worktree \
  --idea "GOALS.md worked well as a project-local operating state file." \
  --requested-change "Add GOALS.md to the template and teach agents to read it first." \
  --acceptance "Generated projects include GOALS.md" \
  --constraint "Do not make unrelated template changes"
```

By default, the briefing is written to:

```text
<target-project>/.codex/handoffs/<timestamp>-<slug>.md
```

## Kick off a target Codex session

```bash
codex-handover kickoff \
  --briefing /path/to/briefing.md \
  --target-project /path/to/target \
  --sandbox read-only
```

By default, `kickoff` uses `--runtime desktop`, which creates a Codex Desktop-loadable thread through `codex app-server`. The command returns JSON containing the new session id, `thread_url`, a `pickup_command`, and the last-message path. Use `--runtime exec` only when a CLI-only handover is acceptable.

Always surface the `codex://threads/<session-id>` deep link so the user can open it manually. Keep `pickup_command` as the CLI fallback for `--runtime exec`, because `codex exec` sessions may not render cleanly in Codex Desktop.

Use `--sandbox workspace-write` only when the handover should immediately implement changes. Use `read-only` for smoke tests, planning, or requirements analysis.

## Pick up or open the session

```bash
codex-handover open <session-id>
codex-handover resume <session-id> "Continue implementing the handover."
```

If the deep link shows a loading screen, use the returned `pickup_command` or `codex-handover resume <session-id> ...`.

## Agent behavior

When using a handover:

1. Read the full briefing before editing.
2. Inspect the target project.
3. Respect the selected implementation mode.
4. Keep the work scoped to the requested change.
5. Verify the result before reporting back.

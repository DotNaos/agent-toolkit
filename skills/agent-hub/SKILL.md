---
name: agent-hub
description: Run and operate the Agent Hub web application for realtime multi-agent group chat with human-in-the-loop approvals.
---

# agent-hub

Use this skill when you need to start or operate the Agent Hub web UI instead of terminal chat loops.

## Run

```bash
./scripts/agent-hub-dev.sh
```

Then open <http://127.0.0.1:5173>.

## What it provides

- Group conversations (`owner` + multiple agents)
- Realtime updates over SSE
- Human-in-the-loop approval modal
- Multiple-choice and free-text approval inputs
- Risk-gated dispatches (`read` auto, risky actions block for approval)

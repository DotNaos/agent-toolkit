---
name: agent-delegate
description: Delegate a one-shot task to a local external agent CLI such as Gemini, Claude, Copilot, or Codex through the unified agent-delegate contract.
---

# agent-delegate

Use this skill when another local model or agent CLI is a better fit for a subtask and you want a strict one-shot delegation boundary instead of switching tools manually.

The model lists exposed by `agent-delegate` are the repo's configured allowlist for each adapter. They are not guaranteed to be a complete live discovery of every remotely available provider model.

Do not trust your internal model memory when choosing a delegated model. The agent's knowledge cutoff can be many months behind the currently available CLI models, so a model that feels "latest" from memory may already be outdated or unavailable. Always inspect the currently configured adapter/model list first, then choose from that live list.

## What it does

- Routes one task to one explicit adapter: `gemini`, `claude`, `copilot`, or `codex`
- Supports explicit model selection per adapter through `model`
- Runs inside the specified `cwd` or the caller's current working directory by default
- Uses curated context as prompt guidance: inline notes plus explicit files/snippets
- Returns one normalized JSON result with status, final text, stderr/stdout, timing, and optional artifacts
- Blocks risky or `guarded_execution` requests unless approval has already been granted by the caller

## CLI

Go wrapper:

```bash
go build -o bin/agent-delegate ./cmd/agent-delegate
./bin/agent-delegate list-adapters --config agent-delegate.json
```

Direct TypeScript implementation:

```bash
cd tools/agent-delegate
bun run src/index.ts list-adapters --config ../../agent-delegate.json
```

Before choosing a model for a delegated run, always inspect the current list:

```bash
./bin/agent-delegate list-adapters --config agent-delegate.json
```

Default model-selection rule:

- Always choose from the current `list-adapters` output, not from memory.
- Prefer the newest strong general-purpose model available for the selected adapter.
- If the adapter exposes a `default_model`, use that unless the task clearly needs a different configured model.
- For `copilot`, consider the configured `multiplier` when two models are otherwise similarly suitable.
- Avoid `Claude Opus` by default because it is expensive; only choose it for unusually important, difficult, or high-stakes tasks where the extra cost is justified.

## Request shape

Send JSON to `run`:

```json
{
  "adapter": "gemini",
  "model": "gemini-2.5-pro",
  "task": "Review this landing page copy and suggest a stronger hero section.",
  "mode": "advisory",
  "cwd": "/absolute/project/path",
  "context": [
    { "type": "inline", "label": "Goal", "text": "Target audience is enterprise buyers." },
    { "type": "file", "path": "web/agent-hub/src/App.tsx" }
  ],
  "metadata": {
    "action": "read"
  }
}
```

Run it:

```bash
./bin/agent-delegate run --request request.json --json
```

Or via stdin:

```bash
cat request.json | ./bin/agent-delegate run --request - --json
```

## When to choose each adapter

- `gemini`: web UI concepts, visual direction, frontend ideation
- `claude`: structured writing, careful code analysis, detailed critique
- `copilot`: multi-model fallback where GitHub Copilot subscriptions expose strong model choices; config can also annotate each model with a `multiplier`
- `codex`: OpenAI-native repo work when staying inside the Codex ecosystem is preferable

## Guardrails

- By default, delegated agents run directly inside the target working directory and can read/write there.
- Prefer setting `cwd` explicitly when the work should be constrained to a specific subdirectory.
- Pass only the files actually needed for the subtask.
- Treat `guarded_execution` as an explicit approval decision, not as the only mode that allows edits.
- Keep delegation narrow. Do not offload the entire parent task when only one specialized subproblem needs another model.
- Never assume a newly released model exists just because it is mentioned in your internal knowledge. Check `list-adapters` first every time model choice matters.

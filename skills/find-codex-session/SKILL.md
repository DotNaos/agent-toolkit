---
name: find-codex-session
description: Find the current or most relevant local Codex session fast, including the session JSONL path, shell snapshots, and recent thread names. Use when handing work off to another tool like GitHub Copilot, when you need to recover the current Codex thread, or when you want the latest session file to attach or inspect.
---

# Find Codex Session

Use this skill when you need the local file that backs a Codex conversation.

The normal workflow is:

1. Start with the freshest 5 sessions from the local Codex index.
2. Sort those fresh sessions by thread-name relevance to the requested session.
3. Read the real session transcripts for those fresh candidates and compare them against the request plus current chat context.
4. If that still does not produce a transcript-backed hit, fall back to a broader transcript keyword search.
5. If no transcript evidence is found, say so plainly instead of returning a weak guess.

## Quick commands

Find the best match for a thread name:

```bash
python3 /Users/oli/projects/agent-toolkit/skills/find-codex-session/scripts/find_codex_session.py --query "Todo Graph"
```

Include current chat context to disambiguate fresh sessions:

```bash
python3 /Users/oli/projects/agent-toolkit/skills/find-codex-session/scripts/find_codex_session.py \
  --query "Todo Graph" \
  --context "we were refining the session-finding heuristic" \
  --context "recent 5 sessions first, then transcript fallback"
```

Show the most recent sessions:

```bash
python3 /Users/oli/projects/agent-toolkit/skills/find-codex-session/scripts/find_codex_session.py --recent
```

Return JSON for automation:

```bash
python3 /Users/oli/projects/agent-toolkit/skills/find-codex-session/scripts/find_codex_session.py --query "Todo Graph" --json
```

Inspect more transcript candidates before deciding:

```bash
python3 /Users/oli/projects/agent-toolkit/skills/find-codex-session/scripts/find_codex_session.py --query "Todo Graph" --candidate-limit 30
```

## What it searches

- `~/.codex/session_index.jsonl`
- `~/.codex/sessions/**/rollout-*.jsonl`
- `~/.codex/shell_snapshots/<session-id>.*.sh`

## Notes

- The session index is only the first filter. It is not the proof.
- The script first checks the freshest 5 sessions, because the active thread is usually recent when the skill is invoked.
- The script can compare transcript contents against both the requested query and extra `--context` snippets from the current chat.
- If the fresh-session pass misses, the script falls back to broader transcript inspection.
- If there is no transcript evidence, the script returns no match instead of pretending it found one.
- The script uses `~/.codex/session_index.jsonl` to narrow the search, then reads the actual transcript under `~/.codex/sessions/...`.
- The session index can contain multiple names for the same session id if the thread got renamed. The script keeps the newest name.
- Prefer the concrete `session_file` in `~/.codex/sessions/...` when you want to attach or inspect the conversation transcript.
- Shell snapshots are useful extra context, but they are not the main transcript.
- When `--query` is used, the output includes transcript evidence snippets so the match is justified by real conversation content, not only by the thread name.

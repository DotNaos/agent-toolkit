---
name: agentation
description: Add Agentation to React and Vite apps, wire the local MCP sync server, and verify that browser annotations reach the agent. Use when setting up Agentation, repairing a broken Agentation setup, or capturing the repeatable Agentation workflow in a repo.
---

# Agentation

Use this skill when a repo needs Agentation added or repaired.

Keep the setup small and repeatable.
Do not scatter one-off notes across the repo.

## Default Target

Default to this shape unless the repo clearly needs something else:

- React app
- Vite dev server
- `agentation` mounted only in development
- `agentation-mcp` available as a local command
- browser toolbar pointing at `http://127.0.0.1:4747`

## Workflow

1. Find the real app root.
2. Install the official packages:
   - `agentation`
   - `agentation-mcp`
3. Mount the toolbar at the top of the app tree, not inside a feature component.
4. Keep it dev-only.
5. Point the toolbar at a local MCP HTTP endpoint.
6. Add local package scripts for:
   - init
   - server
   - doctor
7. Start the MCP server.
8. Start the app.
9. Verify the toolbar appears and the MCP server is healthy.

## React + Vite Pattern

Use a tiny wrapper component and lazy-load it only in development.

Preferred shape:

- app renders normally in production
- development loads a small `AgentationToolbar` component
- toolbar uses `import.meta.env.VITE_AGENTATION_ENDPOINT`
- fallback endpoint is `http://127.0.0.1:4747`

## Package Scripts

Add these scripts when the app uses npm-compatible package scripts:

```json
{
  "agentation:init": "agentation-mcp init",
  "agentation:server": "agentation-mcp server",
  "agentation:doctor": "agentation-mcp doctor"
}
```

## MCP Setup

Official local server command:

```bash
agentation-mcp server
```

Official health check:

```bash
agentation-mcp doctor
```

For tools that need explicit MCP registration, register the same server command they can launch locally.

Claude Code example from the official package:

```bash
claude mcp add agentation -- npx agentation-mcp server
```

## Verification

Do not stop after editing files.

Always verify all of this:

1. package install succeeds
2. app build still succeeds
3. MCP server starts
4. `http://127.0.0.1:4747/health` returns healthy
5. dev app loads in the browser
6. the Agentation toolbar is visible in development

## Notes

- Mount Agentation once, near the root.
- Do not ship it as a production feature.
- Do not hardcode remote endpoints for local setup.
- Prefer one small wrapper file over repeated inline setup.

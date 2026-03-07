# ui-loop

Use this skill to map macOS UI and execute UI actions by semantic element IDs.

## Build

```bash
go build -o bin/ui-loop ./cmd/ui-loop
```

## Requirements

- macOS Accessibility permission enabled for your terminal/Codex app.
- `cliclick` installed (`brew install cliclick`).
- Xcode command-line tools (`xcrun swiftc`).

## Workflow

1. Map current UI:

```bash
ui-loop map-ui
```

2. Inspect IDs from state:

```bash
jq '.elements[] | {id, role, title, value, bounds}' ~/.agent-toolkit/current_view.json
```

3. Click a mapped element:

```bash
ui-loop click --id X002
```

4. Type into a mapped element:

```bash
ui-loop type --id X003 --text "Hallo Welt" --submit
```

All commands return strict JSON responses for agent automation.

# UI Loop Skill

This tool closes the Perception -> Action loop on macOS:
1. `map-ui` captures screenshot + AX tree and writes a structured state file.
2. Agent chooses an element ID (`X001`, `X002`, ...).
3. `click` or `type` executes actions via `cliclick` using center coordinates from the mapped bounds.

## Requirements

- macOS with Accessibility permission enabled for your terminal/Codex app.
- `cliclick` installed and in `PATH`.
- Go 1.22+ and Xcode command line tools (`xcrun swiftc`).

## Build

```bash
go mod tidy
go build -o bin/ui-loop .
```

## Workflow

### 1) Map current UI

```bash
./bin/ui-loop map-ui
```

Default outputs:
- State JSON: `~/.agent-toolkit/current_view.json`
- Screenshot: `~/.agent-toolkit/current_view.png`

### 2) Inspect IDs

```bash
jq '.elements[] | {id, role, title, value, bounds}' ~/.agent-toolkit/current_view.json
```

### 3) Click by element ID

```bash
./bin/ui-loop click --id X002
```

Returns strict JSON, for example:

```json
{"status":"success","action":"click","id":"X002","coords":{"x":100,"y":200}}
```

### 4) Type into an element ID

```bash
./bin/ui-loop type --id X003 --text "Hallo Welt" --submit
```

Behavior:
1. Click center of `X003`.
2. Waits `100ms` (default; override with `--delay-ms`).
3. Types with `cliclick t:"..."`.
4. Optional Enter with `--submit`.

Returns JSON:

```json
{"status":"success","action":"type","id":"X003","coords":{"x":100,"y":200},"text":"Hallo Welt","submitted":true}
```

## Useful flags

- `map-ui --state-file /path/state.json --screenshot /path/view.png`
- `map-ui --axdump-source tools/axdump/axdump.swift --axdump-bin ~/.agent-toolkit/bin/axdump`
- `click --state-file /path/state.json`
- `type --state-file /path/state.json --delay-ms 150`

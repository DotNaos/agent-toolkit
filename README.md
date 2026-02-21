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

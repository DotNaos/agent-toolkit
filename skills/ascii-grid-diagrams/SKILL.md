---
name: ascii-grid-diagrams
description: Render validated ASCII grid diagrams and PNG previews from structured specs. Use when creating roadmap-style ASCII layouts, merged grid cells, pipe or dependency routing, or when hand-written ASCII must stay wall-safe and deterministic.
---

# ASCII Grid Diagrams

Use this skill when an ASCII diagram needs to be correct, repeatable, and easy to revise.

Do not hand-edit large diagrams in chat. Instead:

1. Write or update a JSON spec.
2. Run the formatter.
3. Run the renderer.
4. Inspect the ASCII and PNG output.
5. Adjust the spec, not the final diagram text.

## Model

- The diagram is a fixed-width grid.
- Every row is made of cells that fully cover the declared column count.
- Cells can span multiple columns.
- Pipes never go through walls. A pipe crossing replaces the wall segment at the cell-center.
- Pipe rows are normal grid rows, not free space.

## Files

- Renderer:
  [`scripts/render_ascii_diagram.py`](/Users/oli/projects/agent-toolkit/skills/ascii-grid-diagrams/scripts/render_ascii_diagram.py)
- Example spec:
  [`assets/examples/roadmap-grid.json`](/Users/oli/projects/agent-toolkit/skills/ascii-grid-diagrams/assets/examples/roadmap-grid.json)

## Run

```bash
/Users/oli/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 \
  /Users/oli/projects/agent-toolkit/skills/ascii-grid-diagrams/scripts/render_ascii_diagram.py \
  --spec /Users/oli/projects/agent-toolkit/skills/ascii-grid-diagrams/assets/examples/roadmap-grid.json \
  --format-spec-out /tmp/roadmap-grid.formatted.json \
  --ascii-out /tmp/roadmap-grid.txt \
  --png-scale 3 \
  --png-out /tmp/roadmap-grid.png
```

Useful flags:

- `--stdout` prints the ASCII to the terminal.
- `--format-spec-out` writes a canonical spec with sorted fields and snapped row lanes.
- `--png-scale` increases PNG resolution. Use `3` or `4` for crisp previews in chat.
- `--png-out` writes a PNG preview for attachment.

## Spec shape

Each row contains explicit cells with:

- `lane`: optional row pipe lane: `upper`, `center`, or `lower`
- `col`: zero-based start column
- `span`: optional column span, default `1`
- `kind`: `content`, `pipe`, or `blank`
- `label`: optional centered text
- `edges`: optional list of pipe crossings on `n`, `e`, `s`, `w`
- `interior`: optional pipe shape inside the cell

Supported `interior` values:

- `empty`
- `vertical`
- `horizontal`
- `cross`
- `tee-n`
- `tee-e`
- `tee-s`
- `tee-w`
- `turn-ne`
- `turn-nw`
- `turn-se`
- `turn-sw`

## Constraints the renderer enforces

- row coverage must be complete
- spans may not overlap
- every interior pipe crossing must be matched by the neighboring cell
- outer boundaries may not declare pipe crossings
- pipe rows are snapped to a fixed horizontal lane so routing stays visually aligned

When a diagram looks wrong, fix the JSON spec and rerun the script.

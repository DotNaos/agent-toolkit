---
name: ascii-diagramming
description: General-purpose ASCII diagramming tool. Format raw ASCII, keep spacing rectangular, and render crisp PNG previews from direct text, text files, or an optional structured box-and-wire JSON helper.
---

# ASCII Diagramming

Use this skill when an ASCII diagram needs to be cleaned up, normalized, and turned into a readable image.

This skill is meant to be general-purpose.
Do not rewrite the script for each diagram or project.

The normal input is:

- raw ASCII passed with `--text`
- or a text file passed with `--text-file`

The structured JSON mode is only an optional helper for quick generation.
It is not the default workflow.

## Default workflow

1. Write the ASCII diagram normally.
2. For short diagrams, pass it with `--text`.
3. For larger diagrams, save it to a text file and pass that path with `--text-file`.
4. Let the script pad and normalize the lines.
5. Render a PNG preview.

This is the mode you should reuse in projects like `Todo Graph`.
The usual flow there is: build the ASCII in a normal text file, run the tool, inspect the PNG, and iterate on the text file rather than patching the renderer.

Preferred rule:

- small one-off diagram: `--text`
- anything bigger or multi-line: `--text-file`

Do not change the renderer just because a new diagram has different content.
Only change the renderer when the formatting or rendering behavior itself is wrong.

## Good fits

Use this tool for things like:

- box-and-arrow sketches
- roadmap or dependency diagrams
- trees and branching flows
- small UI wireframes made from ASCII
- hand-written diagrams that need consistent spacing before sharing

Not every diagram needs the structured helper.
If you already have acceptable ASCII, the main value here is formatting plus PNG rendering.

## Agent workflow

When another agent uses this skill:

1. Create the ASCII normally in a temp text file if it is more than a couple of lines.
2. Run the script with `--text-file`.
3. Save both:
   - formatted ASCII
   - preview image
4. Attach the PNG if a visual check or chat attachment is useful.

If the diagram is tiny, `--text` is acceptable, but `--text-file` is the safer default for anything non-trivial.

By default the script writes timestamped files into:

- [`skills/ascii-diagramming/.artifacts/`](/Users/oli/projects/agent-toolkit/skills/ascii-diagramming/.artifacts)

This avoids dumping raw files into `/tmp` and avoids overwriting older renders.

For iterative editing on one fixed file:

- use `--text-file <path>`
- add `--in-place`

That overwrites the same text file with formatted ASCII and, unless overridden, writes the PNG next to it using the same basename.

## Files

- Renderer:
  [`scripts/render_ascii_diagram.py`](/Users/oli/projects/agent-toolkit/skills/ascii-diagramming/scripts/render_ascii_diagram.py)
- Optional structured example spec:
  [`assets/examples/roadmap-grid.json`](/Users/oli/projects/agent-toolkit/skills/ascii-diagramming/assets/examples/roadmap-grid.json)

## Common usage

Format a text file and render a PNG:

```bash
/Users/oli/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 \
  /Users/oli/projects/agent-toolkit/skills/ascii-diagramming/scripts/render_ascii_diagram.py \
  --text-file /tmp/diagram.txt \
  --in-place \
  --png-scale 3 \
  --stdout
```

Pass short raw ASCII directly:

```bash
/Users/oli/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 \
  /Users/oli/projects/agent-toolkit/skills/ascii-diagramming/scripts/render_ascii_diagram.py \
  --text $'Title\n\n+--+\n|A |\n+--+' \
  --stdout
```

Use the optional structured box-and-wire helper:

```bash
/Users/oli/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/bin/python3 \
  /Users/oli/projects/agent-toolkit/skills/ascii-diagramming/scripts/render_ascii_diagram.py \
  --spec /Users/oli/projects/agent-toolkit/skills/ascii-diagramming/assets/examples/roadmap-grid.json \
  --format-spec-out /tmp/roadmap-grid.formatted.json \
  --ascii-out /tmp/roadmap-grid.txt \
  --png-scale 3 \
  --png-out /tmp/roadmap-grid.png
```

## Main flags

- `--text` takes raw ASCII directly.
- `--text-file` reads ASCII from a file path.
- `--artifact-dir` sets the output folder for timestamped generated files.
- `--artifact-stem` sets the base filename before the timestamp.
- `--in-place` overwrites `--text-file` and writes the PNG next to it by default.
- `--ascii-out` overrides the default text output path.
- `--stdout` prints the formatted ASCII.
- `--png-out` overrides the default PNG output path.
- `--png-style pixel` renders a colored, blocky preview instead of the default monochrome line preview.
- `--png-scale` increases PNG resolution. Use `3` or `4` for crisp previews in chat.
- `--spec` uses the optional structured JSON helper format.
- `--format-spec-out` writes a canonical structured spec, but only when using `--spec`.

## What gets formatted

For plain text input the script:

- trims only outer empty lines
- keeps inner spaces intact
- pads all diagram rows to the same width
- preserves a `Title` + blank line + body layout if present

## Optional structured JSON mode

The structured mode is only for convenience when the agent wants help generating box-and-wire ASCII quickly.
It should not replace the default `--text` or `--text-file` workflow.

Think of it as a helper for rectangular diagrams with boxes, merged regions, and routed connections.
It is not the definition of the whole skill.

Each row contains explicit cells with:

- `lane`: optional row pipe lane: `upper`, `center`, or `lower`
- `col`: zero-based start column
- `span`: optional column span, default `1`
- `kind`: `content`, `pipe`, or `blank`
- `label`: optional centered text
- `edges`: optional list of pipe crossings on `n`, `e`, `s`, `w`
- `interior`: optional pipe shape inside the cell

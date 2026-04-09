---
name: macos-app-icon-prompt
description: Generate copy-paste-ready prompts for image models to create macOS app icon artwork, then run a human-in-the-loop workflow through Gemini, Figma, Icon Composer, and final app integration.
---

# macOS App Icon Prompt

Use this skill when the user wants a prompt for an image model such as Gemini to create artwork for a macOS app icon, or when the user wants to run the full icon workflow from prompt generation through app integration.

This skill is prompt-first and workflow-driven.

Do not explain image-generation theory.
Do not add extra commentary unless the user explicitly asks for it.
Default to giving the user something they can paste directly into the image model.

## Output Format

Unless the user asks for something else, return exactly this shape:

```md
{{prompt}}
```

[Open in Gemini](https://gemini.google.com/app)

Rules:

- return exactly one Markdown code block
- put the full copy-paste payload inside that one code block
- the payload should include both the main prompt and the negative prompt
- do not add notes
- do not add explanation before the code block
- do not add explanation after the Gemini link

## Prompt Requirements

By default, make the prompt specific, visual, and directly usable.

Always make sure the prompt asks for:

- a `1:1 aspect ratio`
- a square composition
- one centered object
- comfortable padding around the object
- a clean background that is easy to separate later

Unless the user asks otherwise, bias toward:

- premium macOS icon craftsmanship
- calm Apple-like polish
- a single icon object
- restrained colors
- no mockup
- no text
- no UI
- no app-store tile
- no extra scene dressing

## Human-in-the-Loop Workflow

When the user wants the full workflow, follow these steps:

1. Write the prompt.
2. Copy it to the clipboard with `pbcopy`.
3. Open Gemini immediately in the browser.
4. The user pastes the prompt into Gemini and generates images.
5. Iterate on the prompt without reopening the browser each time.
6. Once the user downloads a result, assume it lands in `~/Downloads`.
7. Open Figma in the browser for the user to do manual background cleanup and upscaling there.
8. Open Icon Composer on macOS for the cleaned export.
9. The user saves the Icon Composer project under the project `assets/` folder and exports the final icon there.
10. Wire the finished icon into the app.

If the user wants a faster Icon Composer setup:

11. Use the bundled template at `assets/icon-composer/icon.json.template`.
12. Use the bundled script at `scripts/create_icon_composer_project.py` to generate a clean `.icon` project from the cleaned PNG.
13. Prefer this script over hand-building the `.icon` bundle when the user wants speed and repeatability.

## Workflow Rules

- do not create a helper script in the repo for this workflow unless the user explicitly asks for one
- when opening Figma for cleanup, also open Finder in the folder that contains the downloaded image so the user can drag it in
- when opening Icon Composer, also open Finder in the folder that contains the cleaned export so the user can drag it in
- whenever the user needs to drag a file into another app, open the relevant Finder folder too
- after the Icon Composer export, rename random PNG filenames in `assets/` to a clean stable name before wiring the icon into the app
- when bootstrapping Icon Composer from a cleaned PNG, create the `.icon` bundle outside the repo only if the user asks; otherwise prefer the project `assets/` folder
- after generating the `.icon` bundle, open both the `.icon` project and its Finder location so the user can export immediately

## Bundled Icon Composer Bootstrap

Use the bundled script when the user has already cleaned the PNG and wants to jump straight into Icon Composer.

Command:

```bash
python3 /Users/oli/projects/agent-toolkit/skills/macos-app-icon-prompt/scripts/create_icon_composer_project.py \
  --source ~/Downloads/cleaned-icon.png \
  --output /absolute/path/to/project/assets/MyApp.icon \
  --name "My App" \
  --open
```

Behavior:

- copies the cleaned PNG into the `.icon` bundle with a stable filename
- writes `icon.json` from the bundled template
- opens the generated `.icon` project in Icon Composer
- reveals the project in Finder

Use `--force` only when replacing an existing `.icon` bundle intentionally.

## Strong Default Prompt Pattern

Use this structure unless the user asks for another direction:

```text
Prompt:
Create a premium macOS app icon object for "[APP NAME OR CONCEPT]".

Show a single centered symbol only: [SYMBOL DESCRIPTION].

Style:
- Apple-like macOS icon craftsmanship
- calm, polished, premium finish
- gentle depth
- clean rounded edges
- crisp silhouette
- restrained material treatment

Color:
- restrained palette
- low saturation
- mostly off-white, soft gray, muted blue-gray
- tiny accent color only if needed

Composition:
- 1:1 aspect ratio
- square composition
- centered object
- comfortable padding
- single floating object
- no app icon tile
- no pedestal
- no scene
- no mockup
- no text
- no watermark

Background:
- perfectly uniform solid black background
- no gradient
- no texture
- no haze
- no vignette
- no background shadow
- easy to separate later

Negative prompt:
futuristic, sci-fi, holographic, neon glow, rainbow colors, app icon tile, rounded square base, pedestal, scene, mockup, text, watermark, UI, extra objects, clipart, cartoon, cheap 3D
```

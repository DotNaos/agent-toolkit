---
name: macos-app-icon-prompt
description: Generate copy-paste-ready prompts for image models to create glossy macOS app icon artwork, especially icon objects that will later be cleaned up, background-removed, or imported into Apple Icon Composer.
---

# macOS App Icon Prompt

Use this skill when the user wants a prompt for an image model such as Gemini, Midjourney, or another image generator to create artwork for a macOS app icon.

This skill is prompt-only.

Do not explain image-generation theory.
Do not produce implementation advice unless asked.
Default to giving the user something they can paste directly into the image model.

## What This Skill Should Output

By default, return:

1. one main prompt
2. one negative prompt
3. one short note if a plain solid background is better than transparency for later cleanup

If helpful, also provide:

- one shorter backup prompt
- one stricter background-removal variant

## Default Visual Direction

Unless the user asks otherwise, bias toward this style:

- premium macOS app icon artwork
- glossy translucent 3D object
- clean Apple-like polish
- centered single-symbol composition
- no UI
- no text
- no mockup
- no device frame
- no app-store presentation card
- no generic icon tile unless explicitly requested

## Default Background Rule

If the user wants to import the result into Icon Composer or remove the background later:

- prefer a perfectly uniform solid background
- usually pure black
- no gradient
- no texture
- no vignette
- no haze
- no background shadow

Do not ask the model for transparency unless the generator reliably supports it.

## Color Rule

Use a restrained palette unless the user explicitly wants more color.

Good default:

- cyan
- teal
- ice blue
- optional tiny hint of violet

Avoid rainbow palettes unless explicitly requested.

## Default Symbol Rule

Prefer one unified symbol instead of multiple objects.

Good default for productivity or education tools:

- open book merged with a terminal prompt
- command-line symbol embedded into the object, not pasted on top

Avoid obvious logo copying.

## Output Format

Use this exact response shape unless the user asks for something else:

### Prompt

```text
...
```

### Negative prompt

```text
...
```

### Note

One or two short lines.

## Prompt Construction Checklist

When writing the prompt, make sure it clearly specifies:

- the symbol
- the material
- the composition
- the background treatment
- the color restraint
- the output cleanliness

If the prompt is too vague, tighten it.
If the prompt contains conflicting instructions, simplify it.

## Strong Default Prompt Pattern

Use this as the base pattern and adapt it to the user’s symbol:

### Prompt

```text
Create a premium 3D macOS app icon object for "[APP NAME OR CONCEPT]".

Show a single centered symbol only: [SYMBOL DESCRIPTION].

Style:
- glossy translucent glass or acrylic material
- polished Apple-like utility icon feel
- soft internal glow
- subtle reflections
- smooth rounded edges
- elegant layered depth
- crisp silhouette
- premium high-end render

Color:
- restrained palette
- mostly cyan, teal, and icy blue
- optional tiny hint of violet
- not rainbow
- not multicolor

Background:
- perfectly uniform solid black background
- no gradient
- no texture
- no haze
- no vignette
- no extra shadow on the background
- background must be easy to remove later

Composition:
- single floating object
- no rounded square app tile
- no pedestal
- no outer frame
- no scene
- no mockup
- no text
- no watermark
```

### Negative prompt

```text
rainbow colors, too many colors, app icon tile, rounded square base, pedestal, background gradient, textured background, vignette, haze, extra objects, text, letters, watermark, UI, mockup, scene, clipart, flat design, cartoon, cheap neon
```

## When the User Wants a More Specific Result

Adjust these parts:

- symbol:
  replace with the exact concept
- color:
  reduce or redirect the palette
- material:
  glossier, softer, sharper, more glassy, more acrylic
- background:
  black by default, or chroma-key green if the user explicitly wants easier cutout work

## Chroma Key Variant

Only use this if the user explicitly wants easier background removal than black:

```text
Use a perfectly uniform pure chroma-key green background in #00FF00 with no lighting variation, no gradient, and no shadow on the background.
```

## Important Behavior

When using this skill, optimize for:

- direct copy-paste usefulness
- strong visual specificity
- minimal fluff

The user should be able to paste the result into Gemini immediately.

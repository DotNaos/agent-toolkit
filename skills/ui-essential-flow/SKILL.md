---
name: ui-essential-flow
description: Simplify cluttered interfaces by focusing on user flow, essential information, and the next decision the user needs to make.
---

# UI Essential Flow

Use this skill when a user wants to simplify a UI, reduce noise, identify unnecessary data, or reframe a screen around user flow and essential information.

This skill helps the agent turn a cluttered interface into a clearer one by starting from user intent instead of from existing UI elements.

## When to Use

Use this skill when the task involves one or more of the following:

- simplifying a busy screen
- removing distracting or redundant UI elements
- deciding what information is actually needed for the next user action
- restructuring a screen around task flow instead of around data density
- reviewing a UI for unnecessary labels, counters, panels, or metadata

## Steps

1. Identify the primary job of the screen.
2. Write the user flow in 3 to 5 steps.
3. For each step, ask:
    - Where am I?
    - What can I do next?
    - What do I need to know right now?
4. Classify the current UI content into:
    - essential
    - supporting
    - noise
5. Remove or de-emphasize anything that does not help the next decision.
6. Preserve hierarchy, but reduce chrome, labels, stats, and decorative metadata.

## What Usually Counts as Noise

- decorative KPI cards that do not change the next action
- taxonomy labels, moods, categories, or internal tags
- repeated metadata already implied by hierarchy
- marketing-style descriptions on utilitarian screens
- multiple competing navigation surfaces
- redundant file-type badges when the filename already tells the user enough
- footer actions on cards when the whole card is already clickable

## What Usually Stays

- breadcrumb when the user is moving through hierarchy
- page title
- current parent context
- the selectable items needed for the next step
- only the metadata needed to disambiguate similar items

## Output Shape

When analyzing a UI, structure the response like this:

### Unneeded data

List the fields, labels, counters, and panels that do not help the next user decision.

### User flow

Describe the shortest path from landing on the screen to reaching the target content.

### What the user wants to know

Reduce each screen to the few questions the user is trying to answer.

### Minimal UI

State the smallest set of elements needed on the screen.

## Design Heuristics

- Prefer one strong navigation model over several weak ones.
- If hierarchy matters, show it with breadcrumb, grouping, and headings.
- If an element does not help orientation or selection, cut it.
- If a metric does not drive action, remove it.
- For productivity UIs, default to calm density and restrained contrast.
- Make the next click obvious.

## Example

For a course portal:

- overview page:
    - user wants to know which semester contains which course
    - keep breadcrumb, semester headings, course cards
    - remove tutor names, decorative tags, dashboard stats
- course page:
    - user wants to know which week contains which resource
    - keep breadcrumb, course title, week headings, resource cards
    - remove repeated semester metadata, descriptions, extra actions

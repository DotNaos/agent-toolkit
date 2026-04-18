---
name: requirements-ui-redesign
description: Discover and structure UI redesign requirements before changing a website or dashboard. Use when an agent should inventory the current UI, ask targeted questions about the user's desired journey, and turn that into an approved UX/UI brief before implementation.
---

# Requirements UI Redesign

Use this skill when the user wants a website or dashboard UI rebuilt, simplified, or reorganized and the requirements are still fuzzy, incomplete, or mixed with taste-level feedback.

This is a discovery-first workflow.
Do not jump from "make the UI better" straight into code changes.
First make the current state and the target journey explicit.

## Required behavior

- Work in the user's language.
- For a real existing UI, inspect the actual screen first instead of guessing.
- Treat `current UI`, `target journey`, and `redesign brief` as separate artifacts.
- If no current UI exists yet, skip Step 1 and move to Step 2.
- Ask about tasks, order, decisions, and success states before asking about style.
- Ask only a few focused questions at a time. Do not dump a giant questionnaire unless the user asks for it.
- Do not implement until the redesign brief exists and the user had a chance to correct it.

## Deliverables

Produce these artifacts in order:

1. `Current UI inventory`
2. `Target user journey`
3. `Interaction and information rules`
4. `Approved redesign brief`

Use the templates in [references/worksheet.md](references/worksheet.md) when you need a copyable format.

## Step 1: Capture the current state

Goal: make the existing UI explicit.

Write down:

- views or major screens
- visible sections and panels
- navigation elements
- filters, inputs, and controls
- tables, cards, charts, lists, and result areas
- key actions
- empty, loading, error, and success states
- obvious duplication or clutter

Rules:

- record what exists, not what should exist
- separate visible fact from interpretation
- keep the inventory concrete and screen-based
- if multiple views matter, inventory each one separately

The output must be easy for the user to copy and trim.
Use a checklist-style block, not a paragraph.

If no current UI exists:

- state `No current UI to inventory`
- continue to Step 2 immediately

## Step 2: Define the target outcome

Goal: find out what the user actually wants to do in the product.

Ask for the intended user journey in the order the user would expect it to happen.

Prioritize questions like:

- What is the main thing the user wants to accomplish?
- What should they see first?
- What do they need to understand before taking the first action?
- What do they do next, step by step?
- What must stay visible during the flow?
- What can stay hidden until later?
- What would feel confusing, slow, or annoying?
- What should success look like at the end?

Do not start with:

- color palette questions
- animation questions
- aesthetic adjectives without task context

If the user answers loosely, rewrite the answer into a numbered journey and ask for confirmation or correction.

## Step 3: Normalize the journey

Turn the user's raw description into a short, structured flow:

1. entry point
2. first visible information
3. first user decision
4. next action
5. system response
6. completion or success state

For each step, capture:

- what the user wants
- what must be visible
- what action is available
- what can wait or stay hidden

Keep the main path separate from side paths and exceptions.

## Step 4: Define interaction and information rules

Turn the journey into concrete UI rules.

Cover at least:

- primary job of the screen
- primary action per step
- information hierarchy
- what should be always visible
- what should be secondary
- what should be removed
- what should be combined
- required empty, loading, error, and success states
- desktop and mobile differences when relevant

Useful pressure tests:

- Does this element help the next decision?
- Is this shown too early?
- Is the same information repeated elsewhere?
- Is the UI exposing system structure instead of user intent?

## Step 5: Produce the redesign brief

Write a brief that can drive implementation without forcing the user to restate everything.

The brief should contain:

- the main user outcome
- the target user journey
- elements to keep
- elements to remove or de-emphasize
- new elements or states needed
- screen structure and hierarchy
- navigation model
- success criteria
- open questions or assumptions

Do not hide uncertainty.
If something is assumed rather than confirmed, label it clearly.

## Output shape

Use this order when reporting the requirements work:

### Current UI inventory

Concrete checklist of what exists now.

### Target user journey

Short numbered flow in the user's language.

### Interaction and information rules

Explicit rules for visibility, sequence, emphasis, and removal.

### Redesign brief

Concise implementation-ready summary.

## When to combine with other skills

- Use `agent-browser` when you need to inspect a live website or dashboard.
- Use `ui-essential-flow` after requirements are clear and you need to cut noise.
- Use `ui-clarity-loop` while iterating on the actual redesign and reviewing what still feels wrong.

## Guardrails

- Do not confuse the current UI with the target UI.
- Do not confuse user preference with user need.
- Do not keep elements just because they already exist.
- Do not remove elements without checking whether they support a real decision or task.
- Do not present implementation as complete if the requirements artifacts were skipped.

---
name: readme-tutorial-flow
description: Write or rewrite READMEs and project docs as short, action-first step-by-step flows with minimal explanation. Use when the user wants onboarding docs, "idiot proof" setup steps, cleaner tutorials, a full docs structure, or a choice between explaining in chat versus writing files.
---

# README Tutorial Flow

Use this skill when the user wants:

- a README rewritten to be easier to follow
- a setup guide that tells them exactly what to do next
- a full documentation structure for a software project
- onboarding docs with less noise
- an "idiot proof" tutorial
- a choice between a short explanation in chat and writing the tutorial into a file

## Output modes

Pick one mode at the start.

### 1. Reply mode

Use this when the user wants the explanation in the conversation.

Output shape:

- short plain-English explanation
- only the next actions
- no large reference dump

### 2. File mode

Use this when the user wants a README, setup guide, or tutorial written into the repo.

Output shape:

- write the file directly
- keep the main path in the file
- move optional or background material into secondary docs if needed

If the user does not specify, infer the mode:

- if they mention README, tutorial file, docs, or ask you to change repo documentation, use file mode
- if they only ask how to do something, use reply mode

## Core rule

Start with the first action, not with meta-explanation.

Good:

1. Copy `.env.example` to `.env`
2. Run the check command
3. Fill only the missing values

Bad:

- long introductions about philosophy
- explaining what the README is trying to do
- mixing setup steps with architecture notes

## Default project-docs shape

When the task is about the documentation structure of a whole project, use this shape by default:

- `README`
  - title
  - index
  - short abstract
  - getting started
  - guides
  - reference
  - troubleshooting
  - development

### Section roles

- `Abstract`
  - explain the project in under 30 seconds
- `Getting started`
  - move the reader into the normal working state
- `Guides`
  - answer "how do I do X?"
- `Reference`
  - answer "what is the exact fact?"
- `Troubleshooting`
  - answer "what do I do when this fails?"
- `Development`
  - explain how the project works inside and how to change it safely

### Development section

For software projects, `Development` should usually contain:

- `Internals`
- `Architecture`
- `Code structure`
- `Extension points` when relevant
- contributor or verification workflows when relevant

Treat `Development` as a clear boundary between "using the project" and "changing the project".

## What a good tutorial looks like

### 1. Action first

The first lines should help the user move.

Allowed at the top:

- title
- at most 3 short orientation lines

Then go straight to step 1.

### 2. One path only

Write the default path first.

Do not mix these into the main path unless they are needed right now:

- advanced options
- architecture
- background explanation
- alternative workflows
- future plans

Put those at the bottom or in secondary docs.

### 3. Every step answers one question

Each step should make the next action obvious:

- what do I run
- what file do I open
- what should happen
- what do I do if it does not happen

### 4. Explain only when necessary

Prefer:

- "Run this"
- "If you see this, continue"
- "If not, fill these values and run it again"

Avoid:

- unnecessary theory
- tool history
- internal implementation detail
- repeated warnings

### 5. Keep cognitive load low

Prefer:

- short sentences
- one action per step
- explicit filenames and commands
- "only read this if you are stuck"

Avoid:

- large bullet walls
- deep nesting
- multiple decisions inside one step

## File-mode workflow

When writing or rewriting a README or tutorial file:

1. Identify the default success path.
2. Move everything not needed for that path down or out.
3. For project docs, make the README a router: index first, short abstract, then getting started, then the other doc groups.
4. Keep the opening to a title plus up to 3 short lines.
5. Make step 1 immediately actionable.
6. Split the rest of the docs by job:
   - getting started
   - guides
   - reference
   - troubleshooting
   - development
7. Inside `Development`, separate internals from user-facing guides.
8. For each step, include:
   - the command or action
   - what result to expect
   - what to do next
9. Push optional docs into a short "extra docs" section at the end.
10. If a branch is long or optional, move it into a separate Markdown file instead of cluttering the main tutorial.
11. In the main tutorial, only point to the side path.
12. In the side-path file, explain whether the reader should return to the main tutorial, continue to another side path, or stop because they are done.
13. Verify the steps against the real repo or commands if possible.

## Branching for saved tutorials

Use this only for tutorials that are written into files.

Keep the main tutorial linear.

Good pattern:

- main tutorial:
  - step 1
  - step 2
  - "If this fails, continue in `ssh-access.md`."
  - step 3
- side-path tutorial:
  - fix the problem
  - tell the reader whether to return to the main tutorial or continue to another branch

Rules:

- do not put the return logic before the link in the main tutorial
- do put the return logic inside the branch file
- keep short, common exceptions inline only if they fit in a small paragraph
- move long, rare, or disruptive branches into their own file

Use separate files when:

- the branch is optional
- the branch is longer than a short paragraph
- the branch only affects some users
- the branch would break the flow of the main path

## Reply-mode workflow

When answering in chat:

1. Give the shortest working sequence first.
2. Mention only the next step after the current one.
3. Add detail only if the user is blocked or explicitly asks for it.
4. Use plain English, not code-speak, unless commands or paths are needed.

## Tutorial checklist

Before finishing, ask:

- Does the first screenful contain a real first action?
- Can the user follow the default path without opening three other docs?
- Did I remove explanation that does not change the next action?
- Did I separate optional information from the main path?
- If there is a command, did I verify it if possible?

## Common fixes

If the doc feels noisy:

- cut the intro
- merge duplicate explanation
- remove architecture from the setup path
- move long variable explanations into a separate doc
- add a check command early so the user knows what is missing

If the doc feels vague:

- replace "configure" with the exact file or command
- say what success looks like
- say what the next step is

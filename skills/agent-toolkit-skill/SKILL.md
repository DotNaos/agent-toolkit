---
name: agent-toolkit-skill
description: "Create or update a repo-local agent skill inside the current agent-toolkit checkout. Use when the user wants a new skill added under skills/, wants an existing repo skill refined, or says the skill must be created in this repository instead of ~/.agents/skills or other dotfiles. This skill includes the full delivery workflow: create a dedicated git worktree and branch, implement the tracked skill files in the repo, commit, push, and open a GitHub pull request."
---

# Agent Toolkit Skill

Use this as a repo-specific workflow, not as generic skill-creation advice.

## Core rules

- Treat the current Git checkout as the target repository.
- Create or edit delivered skill files only under `skills/<skill-slug>/...` in this repository.
- Never put the delivered skill under `~/.agents/skills`, `~/.config`, or other dotfiles unless the user explicitly asks for that.
- Keep the change scoped to the requested skill plus directly related repo docs, usually `README.md`.

## Default workflow

1. Inspect `skills/` and one or two nearby `SKILL.md` files to match local style.
2. Derive a short, unique skill slug. Prefer the user-provided name; otherwise use a clear hyphenated slug.
3. Create a dedicated worktree and branch before editing.
4. Implement the skill under `skills/<skill-slug>/`.
5. Update repo docs that enumerate shipped skills when needed.
6. Validate the new skill structure and wording.
7. Commit, push, and open a PR.

## Worktree and branch workflow

Prefer the repo's sibling `.worktrees` directory.

- If the current checkout already lives under `/path/to/repo.worktrees/<existing-worktree>`, create the new worktree in that same `/path/to/repo.worktrees` directory.
- Otherwise create or reuse `/path/to/repo.worktrees` next to the canonical repo root.
- Default branch name: `agents/<skill-slug>`
- Default worktree directory: branch name with `/` replaced by `-`

Example:

```bash
repo_root=$(git rev-parse --show-toplevel)
repo_parent=$(dirname "$repo_root")
if [[ $(basename "$repo_parent") == *.worktrees ]]; then
  worktrees_dir="$repo_parent"
else
  worktrees_dir="${repo_root}.worktrees"
  mkdir -p "$worktrees_dir"
fi

skill_slug=<skill-slug>
branch="agents/$skill_slug"
worktree_dir="$worktrees_dir/${branch//\//-}"

git worktree add -b "$branch" "$worktree_dir" HEAD
cd "$worktree_dir"
```

If the branch already exists:

```bash
git worktree add "$worktree_dir" "$branch"
cd "$worktree_dir"
```

## Skill structure

- Required file: `skills/<skill-slug>/SKILL.md`
- Add `scripts/`, `references/`, or `assets/` only when they remove repeated work or capture repo-specific knowledge that would otherwise be rediscovered each time.
- Keep frontmatter minimal: only `name` and `description`.
- Put trigger language in the `description`, not in a "When to use" section.

## Content checklist

- Use a unique skill name that will not clash with generic creator skills.
- Make the description say both what the skill does and when it should trigger.
- Write instructions for another agent, not for a human maintainer.
- Keep repo-specific paths and conventions explicit.
- Use examples that point to tracked repo paths such as `skills/<skill-slug>/SKILL.md`.

## Repo docs

If the README or another tracked file lists shipped skills, add the new skill there in the same style as the existing entries.

## Delivery

After editing, finish the workflow instead of stopping at local files:

```bash
git status --short
git add skills/<skill-slug> README.md
git commit -m "feat(skills): add <skill-slug>

Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
git push -u origin "$branch"
gh pr create --base main --head "$branch" --fill
```

- If `README.md` was not changed, omit it from `git add`.
- Open a normal PR after the skill is ready. Use a draft PR only when the user explicitly wants that or the work is intentionally incomplete.

## Guardrails

- Do not create the delivered skill in the installed global skill directory.
- Do not stop after describing a plan. The target end state is a tracked repo change plus pushed branch plus PR.
- If targeted files already contain unrelated user changes, stop and ask before overwriting them.

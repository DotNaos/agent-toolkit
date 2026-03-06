---
name: repo-branch-protection
description: Standardize GitHub branch protection (main + dev), PR-first workflow, and hotfix flow using GitHub CLI.
---

# repo-branch-protection

Use this skill when you want a fast, repeatable baseline for repository branch safety:

- Protect `main` so direct pushes are blocked by required PR reviews.
- Disallow force pushes on protected branches.
- Optionally create and protect `dev` for staging/testing flow.
- Keep hotfixes simple and low-friction.

## Prerequisites

- GitHub CLI installed (`gh --version`).
- Authenticated CLI session with `repo` scope (`gh auth status`).
- Repo owner/name known (for example `DotNaos/Aryazos`).

## 1) Verify current branch state

Check default branch and whether protection already exists.

```bash
gh repo view <owner>/<repo> --json nameWithOwner,defaultBranchRef
gh api repos/<owner>/<repo>/branches/main/protection
```

If branch protection is missing, GitHub returns `404 Branch not protected`.

## 2) Protect `main` (production baseline)

Use this baseline:

- PR review required (`required_approving_review_count: 1`)
- force push disabled
- branch deletion disabled
- admins also enforced

```bash
gh api --method PUT repos/<owner>/<repo>/branches/main/protection --input - <<'JSON'
{
  "required_status_checks": null,
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": false,
  "lock_branch": false,
  "allow_fork_syncing": false
}
JSON
```

Verify:

```bash
gh api repos/<owner>/<repo>/branches/main/protection
```

## 3) Create `dev` branch (optional, recommended)

Create `dev` from current `main` commit if it does not exist:

```bash
MAIN_SHA=$(gh api repos/<owner>/<repo>/git/ref/heads/main --jq '.object.sha')
gh api repos/<owner>/<repo>/git/refs -f ref='refs/heads/dev' -f sha="$MAIN_SHA"
```

Verify:

```bash
gh api repos/<owner>/<repo>/branches/dev --jq '{name: .name, protected: .protected, commit: .commit.sha}'
```

## 4) Protect `dev` (staging baseline)

Suggested defaults for speed + safety:

- PR review required (`1` approval)
- force push disabled
- branch deletion disabled
- admin enforcement optional (often `false` for faster staging ops)

```bash
gh api --method PUT repos/<owner>/<repo>/branches/dev/protection --input - <<'JSON'
{
  "required_status_checks": null,
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": false,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 1,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": false,
  "lock_branch": false,
  "allow_fork_syncing": false
}
JSON
```

Verify:

```bash
gh api repos/<owner>/<repo>/branches/dev/protection
```

## 5) Recommended workflow after setup

### Day-to-day

- `feature/*` -> PR into `dev`
- test/deploy from `dev` to staging
- PR `dev` -> `main` for production release

### Hotfix flow

1. Branch from `main`: `hotfix/<short-name>`
2. PR into `main`
3. Deploy production
4. Back-merge `main` -> `dev` (or cherry-pick) to prevent drift

## Low-friction PR tips

- Keep branch scope small and focused.
- Use squash merge for cleaner history.
- Use draft PRs early, mark ready when checks pass.
- Keep required reviewers to `1` on `dev`.

## Quick checklist

- `main` protected and force push disabled
- `dev` created and protected
- PR-first policy agreed by team
- hotfix back-merge rule documented

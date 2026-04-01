# BattleStream Git Workflow Policy

## Purpose

This document defines the standard workflow all developers, UI engineers, and QA agents must follow when working on independent issues in the BattleStream project. It ensures no work is ever done directly on `main`, that QA always validates the exact code that will be merged, and that the repo history stays clean.

---

## Core Rules

1. **Never commit directly to `main`.** All work — features, bug fixes, UI changes, QA tooling — must live on a dedicated branch or worktree.
2. **Every issue gets its own git worktree.** Worktrees allow parallel development without branch-switching overhead and prevent one agent's work from affecting another.
3. **QA uses the same worktree as the developer.** This guarantees QA is testing exactly the code that will be merged — no re-checkout, no divergence.
4. **Merging to `main` requires QA sign-off.** A task cannot be marked `done` and merged until QA has validated it on the worktree.
5. **`main` is always releasable.** Only clean, tested, QA-approved code lands on `main`.
6. **Bug fixes must include a regression test.** Every bug fix must be accompanied by a test that reproduces the original failure and passes after the fix. This prevents the same bug from regressing silently.
7. **QA-ready issues must be assigned to the QA engineer.** When a task is ready for review, set the Paperclip issue status to `in_review` and **assign it to the QA agent**. Do not leave it assigned to yourself. QA will not pick up tasks that are not explicitly assigned to them.

---

## Workflow Step-by-Step

### 1. Starting an Issue (Developer)

When you pick up a task, create a git worktree for it:

```sh
# From the repo root
git fetch origin
git worktree add ../battlestream-<issue-id> -b feat/<issue-id> origin/main
cd ../battlestream-<issue-id>
```

- Name the worktree directory after the issue identifier (e.g. `battlestream-BAT-7`).
- Branch off the latest `origin/main`.
- All commits for this issue go on this branch.

### 2. Development

- Do all dev work inside the worktree directory.
- Commit frequently with meaningful messages. Always include:
  ```
  Co-Authored-By: Paperclip <noreply@paperclip.ing>
  ```
- Push the branch to remote at natural checkpoints:
  ```sh
  git push -u origin feat/<issue-id>
  ```
- Run `go vet ./...` before every commit.
- Do not merge into `main` yourself. Do not rebase onto `main` mid-flight unless explicitly coordinated with the team.

#### Bug Fix Requirement

If the issue is a **bug fix**, you must include a regression test before handing off to QA:

1. Write a test that **fails** against the unfixed code, reproducing the original bug.
2. Apply the fix so the test passes.
3. Confirm the full test suite still passes: `go test -race -count=1 ./...`

Do not submit a bug fix for QA review without a corresponding regression test. QA will reject fixes that lack one.

### 3. Handing Off to QA

When development is complete:

1. Ensure all changes are committed and pushed:
   ```sh
   git status   # must be clean
   git push
   ```
2. Update the Paperclip issue to `in_review` with a comment that includes:
   - What was built and how to test it
   - The worktree path (e.g. `../battlestream-BAT-7`) or branch name
3. **Assign the issue to the QA engineer** (required — QA will not act on tasks not assigned to them).

QA uses the **same worktree** the developer created — no separate checkout.

### 4. QA Validation

QA agent:

1. Navigates to the existing worktree directory (path from the dev handoff comment).
2. Pulls latest from the feature branch:
   ```sh
   git pull
   ```
3. Runs the full validation suite.
4. Records results in a Paperclip comment on the issue.
5. **Pass** → marks issue `done`, notifies developer.
6. **Fail** → sets issue back to `in_progress`, assigns to developer with a clear bug report.

### 5. Merging to `main`

Once QA passes:

1. From the repo root (on `main`):
   ```sh
   git fetch origin
   git merge --ff-only feat/<issue-id>
   git push origin main
   ```
2. CI must be green before and after push.
3. Clean up the worktree:
   ```sh
   git worktree remove ../battlestream-<issue-id>
   git branch -d feat/<issue-id>
   git push origin --delete feat/<issue-id>
   ```

---

## Worktree Quick Reference

| Command | Purpose |
|---|---|
| `git worktree add ../battlestream-<id> -b feat/<id> origin/main` | Create new worktree from main |
| `git worktree list` | Show all active worktrees |
| `git worktree remove <path>` | Remove worktree when done |
| `git worktree prune` | Clean up stale worktree refs |

---

## What NOT to Do

- Do not `git checkout` a different branch inside a worktree — each worktree owns its branch exclusively.
- Do not create a worktree from a branch already checked out in another worktree.
- Do not commit `.claude/plans/`, screenshot files (`*.png`), or game log directories (`Hearthstone_*/`) — these are gitignored.
- Do not push directly to `main`. Even hotfixes go through a branch.
- Do not mark a task `done` in Paperclip before QA has signed off.
- Do not submit a bug fix without a regression test — QA will reject it.
- Do not leave a QA-ready task assigned to yourself — it must be reassigned to the QA engineer with status `in_review`.

---

## Flow Summary

```
New issue
  → git worktree (feat/<id> from origin/main)
  → dev commits + push
  → Paperclip: status=in_review + assign to QA engineer (required)
  → QA validates same worktree
  → pass: merge to main + push + clean up worktree
  → fail: back to in_progress → assign back to dev → fixes → repeat QA
```

# AGENTS.md

Conventions for AI agents (and humans) working in this repo.

## Git Workflow

### Branches

- **Never push to `main`.** It is protected on the remote; pushes will be rejected.
- Before starting a new feature or fix, switch to `main` and pull the latest changes:
  ```bash
  git checkout main && git pull
  ```
- Create a new branch for every change. Use meaningful prefixes mirroring conventional commits:
  - `feat/<short-description>`
  - `fix/<short-description>`
  - `docs/<short-description>`
  - `chore/<short-description>`
  - `refactor/<short-description>`
  - `test/<short-description>`
  - `perf/<short-description>`
  - `ci/<short-description>`
  - `build/<short-description>`

### Commits

- All PRs are **rebase-merged** (the only merge method enabled). Every commit on the branch lands on `main` as-is, so the branch history **is** the project history.
- **Every commit message** must follow Conventional Commits with proper prefixes: `fix:`, `chore:`, `docs:`, `feat:`, `refactor:`, `test:`, `perf:`, `ci:`, `build:`.
- When the change is specific to a feature/module, scope the commit: `fix(auth):`, `feat(parser):`, `docs(readme):`.
- Each commit should be a self-contained, meaningful step that builds and passes tests. No `wip`, `fixup`, or "address review" commits on `main`: squash or reword them (e.g. `git commit --fixup <sha>`, then `GIT_SEQUENCE_EDITOR=: git rebase -i --autosquash origin/main`, which runs non-interactively) before the PR is merged.
- Do not commit everything blindly. Stage only the files relevant to the change.
- Do not touch unrelated files. If you notice something else that needs fixing, open a separate branch/PR.
- Before committing, verify no files/directories that belong in `.gitignore` are being staged (e.g. build output, `.env`, secrets, `.direnv/`, `scratch/`). When unsure, ask.
- Prefer `git stash` over `git checkout --` / `git restore` / `git reset --hard`. Never discard uncommitted work without confirming with the user first.
- Rewriting your own branch's history (rebase, reword, squash, force-push with `--force-with-lease`) is expected, so that the commits are clean before merge. Never force-push to `main` or branches others may be using.

### Pull Requests

- After pushing a new branch, if the `gh` CLI is available, open a PR:
  ```bash
  gh pr create --title "<conventional-commit-style title>" --body "<detailed description>"
  ```
- PR title: conventional-commit style (e.g. `feat(auth): add token refresh`). For a single-commit PR, match the commit message.
- PR body: describe **what** changed, **why**, and **how to test**. Link related issues.
- If the branch is behind `main`, rebase before opening the PR (rebase-merge requires a branch that applies cleanly on `main`):
  ```bash
  git fetch origin && git rebase origin/main
  ```
- **Never merge a PR yourself.** The user reviews and merges.

### After Merge

- Switch to `main`, pull latest, and delete the now-merged local branch:
  ```bash
  git checkout main && git pull
  git branch -d <branch-name>
  ```
  Rebase-merging rewrites the commits, so `git branch -d` may refuse ("not fully merged"). Once you've confirmed the PR is merged, `git branch -D` is fine.
- If the remote branch was not auto-deleted, offer to delete it: `git push origin --delete <branch-name>` (confirm with user first).

## Pre-commit Checks (Go project)

- Run `go build ./...` and `go test ./...` before committing code changes.
- Run `golangci-lint run` if available; the config lives in `.golangci.yml`.
- Run `gofmt -l .` and `goimports -l .` — no output means clean.

## General Rules

- Never force-push or rewrite history on `main` or on branches you did not create / that others may be using. Force-pushing (`--force-with-lease`) to your own short-lived feature branch is fine and is how you clean up its commits before merge.
- Never commit secrets, credentials, or `.env` files.
- When in doubt about an irreversible action (force-push, hard reset, branch deletion, history rewrite), stop and ask the user.

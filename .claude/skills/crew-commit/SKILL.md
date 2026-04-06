---
name: crew-commit
description: >
  Canonical commit workflow for Gas Town crew members: pre-flight checks,
  branch creation, gt commit with agent identity, push, and PR creation.
  Use when ready to commit and submit work for review.
allowed-tools: "Bash(git *), Bash(gt *), Bash(gh *)"
version: "1.0.0"
author: "Gas Town"
---

# Crew Commit — Canonical Git Workflow

This skill guides crew members through the standard Gas Town commit workflow:
pre-flight → branch → stage → commit → push → PR.

> **⚠️ NEVER commit directly to `main`.** All crew work goes through branches
> and pull requests. The Refinery handles merges to main.

## Usage

```
/crew-commit
```

Run this when you have changes ready to commit. The skill walks you through
each step in order.

---

## Step 1: Pre-flight Checks

Before touching anything, sync with origin and verify your state.

```bash
# Fetch latest from origin
git fetch origin

# Check current branch and status
git status
git branch --show-current

# If you're on main, STOP — create a branch first (Step 2)
```

**If you're behind origin/main**, rebase now:

```bash
git rebase origin/main
```

If there are conflicts, resolve them carefully before proceeding.
If stuck on a rebase conflict, stop and get help rather than force-pushing.

---

## Step 2: Create a Feature Branch

**If you're already on a feature branch**, skip to Step 3.

Branch naming convention: `<type>/<short-description>`

Types:
- `feat/` — new feature
- `fix/` — bug fix
- `refactor/` — code restructuring
- `docs/` — documentation only
- `chore/` — maintenance, deps, config
- `test/` — tests only

```bash
# Create and switch to feature branch
git checkout -b feat/my-feature-description

# Or use the crew/<name> prefix for crew-specific branches
git checkout -b crew/<your-name>/description
```

---

## Step 3: Submodule Warning

**Check for submodules before staging.** Accidentally committing a submodule
pointer change causes cascading failures for other crew members.

```bash
# Check if repo has submodules
cat .gitmodules 2>/dev/null || echo "(no submodules)"

# Check for dirty submodule state
git submodule status 2>/dev/null
```

**Common submodule paths to watch:** `shared/`, `config/`, `vendor/`

If you see changes in submodule directories:
- Do NOT `git add shared/` or `git add config/` unless you intentionally
  bumped the submodule pointer
- Submodule pointer changes should be explicit and deliberate
- When in doubt, ask before including submodule changes

---

## Step 4: Stage Your Changes

Prefer staging specific files over `git add .` or `git add -A`.

```bash
# Review what changed
git diff
git status

# Stage specific files (preferred)
git add src/myfile.go tests/myfile_test.go

# If you need to stage all intentional changes and have verified no secrets:
git add -p    # Interactive staging — review each hunk
```

**Before staging, verify:**
- [ ] No `.env` files, API keys, or credentials
- [ ] No debug prints or temporary test code
- [ ] No unrelated changes mixed in
- [ ] Submodule directories NOT accidentally staged (unless intentional)

---

## Step 5: Commit with gt commit

Use `gt commit` instead of `git commit`. It automatically sets the correct
agent identity (name + email) based on your `GT_ROLE`.

```bash
gt commit -m "$(cat <<'EOF'
<type>: <concise description of what and why>

<optional body: context, motivation, or notable details>
EOF
)"
```

**Commit message format:**
- First line: `<type>: <subject>` (50 chars or less)
- Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`
- Use imperative mood: "add feature" not "added feature"
- Body is optional but valuable for non-obvious changes

**Good examples:**
```
feat: add retry logic to webhook delivery
fix: prevent nil pointer when session token expires
docs: clarify crew commit workflow in CONTRIBUTING.md
```

---

## Step 6: Push Branch to Origin

```bash
git push origin <your-branch-name>

# Or, if branch doesn't exist on remote yet:
git push -u origin <your-branch-name>
```

---

## Step 7: Create Pull Request

```bash
gh pr create --title "<type>: <concise description>" \
  --body "$(cat <<'EOF'
## Summary

- <what changed and why>

## Test plan

- [ ] <how to verify this works>

## Notes

<any context reviewers need>

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

After creating the PR, note the PR number from the output.

---

## Step 8: Notify (Optional)

If your work affects others or is high priority:

```bash
notify "PR ready: <brief description> — #<PR number>"
```

---

## Completion Checklist

- [ ] Synced with origin/main (git fetch + rebase)
- [ ] On a feature branch (NOT main)
- [ ] Submodules NOT accidentally staged
- [ ] Specific files staged (no secrets, no debug code)
- [ ] Used `gt commit` (not `git commit`)
- [ ] Branch pushed to origin
- [ ] PR created via `gh pr create`

---

## Anti-Patterns

| ❌ Don't | ✅ Do instead |
|----------|--------------|
| `git push origin main` | Push feature branch, create PR |
| `git commit` directly | `gt commit` (sets agent identity) |
| `git add .` blindly | Stage specific files, verify with `git status` |
| Include `shared/` or `config/` without intent | Check `git submodule status` first |
| Force-push without understanding why | Resolve the root cause |
| Commit secrets or .env files | Always check `git diff` before staging |

---

## If You Get Stuck

- **Rebase conflicts**: resolve carefully, then `git rebase --continue`
- **Pushed wrong branch**: ask before force-pushing; usually `git push origin <branch>` is fine
- **Need to undo last commit**: `git reset HEAD~1` (keeps changes staged)
- **Committed to main by mistake**: stop immediately, ask for help

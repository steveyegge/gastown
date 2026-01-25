---
name: git-ops
description: >
  Common git workflows including commits, branches, PRs, and merge handling.
  Follows Co-Authored-By conventions for AI-assisted commits.
allowed-tools: "Bash(git:*),Bash(gh:*),Read"
version: "1.0.0"
author: "Gas Town"
license: "MIT"
---

# Git Operations - Version Control Workflows

Patterns for git operations, GitHub PRs, and collaborative workflows.

## Co-Authored-By Convention

All AI-assisted commits MUST include the Co-Authored-By trailer:

```bash
git commit -m "$(cat <<'EOF'
Brief description of changes

More details if needed.

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

## Commit Patterns

### Standard Commit

```bash
# Check what changed
git status
git diff

# Stage specific files (preferred over -A)
git add src/file1.go src/file2.go

# Commit with proper message
git commit -m "$(cat <<'EOF'
feat: add user authentication endpoint

Implements JWT-based auth with refresh tokens.
Includes middleware for protected routes.

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

### Commit Message Conventions

- **feat:** New feature
- **fix:** Bug fix
- **docs:** Documentation only
- **style:** Formatting, no code change
- **refactor:** Code change that neither fixes nor adds
- **test:** Adding or fixing tests
- **chore:** Maintenance tasks

## Branch Operations

### Create Feature Branch

```bash
# Ensure main is up to date
git checkout main
git pull origin main

# Create and switch to feature branch
git checkout -b feature/user-auth

# Push with upstream tracking
git push -u origin feature/user-auth
```

### Sync with Main

```bash
# Option 1: Rebase (preferred for linear history)
git fetch origin
git rebase origin/main

# Option 2: Merge (when rebase is complex)
git fetch origin
git merge origin/main
```

## Pull Request Workflow

### Create PR via gh CLI

```bash
# Push current branch
git push -u origin HEAD

# Create PR
gh pr create --title "feat: add user authentication" --body "$(cat <<'EOF'
## Summary
- Implements JWT authentication
- Adds login/logout endpoints
- Includes refresh token rotation

## Test Plan
- [ ] Unit tests pass
- [ ] Manual testing completed
- [ ] No regressions in existing auth

Co-Authored-By: Claude <noreply@anthropic.com>
EOF
)"
```

### View and Check PR Status

```bash
# View PR details
gh pr view

# Check CI status
gh pr checks

# View PR diff
gh pr diff
```

### Merge PR

```bash
# Squash merge (clean history)
gh pr merge --squash

# Regular merge
gh pr merge --merge

# Rebase merge
gh pr merge --rebase
```

## Handling Merge Conflicts

### Identify Conflicts

```bash
git status  # Shows conflicted files

# View conflict markers
grep -rn "<<<<<<" .
```

### Resolve Conflicts

```bash
# Open conflicted file and resolve manually
# Remove conflict markers: <<<<<<<, =======, >>>>>>>

# Mark as resolved
git add resolved-file.go

# Continue operation
git rebase --continue  # if rebasing
git merge --continue   # if merging
```

### Abort if Needed

```bash
git rebase --abort
git merge --abort
```

## Rebase Workflows

### Interactive Rebase

```bash
# Squash last 3 commits
git rebase -i HEAD~3

# In editor, change 'pick' to 'squash' or 's' for commits to combine
```

### Rebase onto Main

```bash
git fetch origin
git rebase origin/main

# Force push after rebase (only for feature branches!)
git push --force-with-lease
```

## Safety Guidelines

1. **Never force push to main/master**
2. **Use --force-with-lease** instead of --force
3. **Stage specific files** instead of `git add -A`
4. **Avoid committing secrets** (.env, credentials)
5. **Always include Co-Authored-By** for AI commits
6. **Never use interactive flags** (-i) in automated contexts

## Quick Reference

| Action | Command |
|--------|---------|
| Status | `git status` |
| Stage | `git add <files>` |
| Commit | `git commit -m "msg"` |
| Push | `git push` |
| Pull | `git pull` |
| Create branch | `git checkout -b <name>` |
| Switch branch | `git checkout <name>` |
| Create PR | `gh pr create` |
| View PR | `gh pr view` |
| Merge PR | `gh pr merge` |

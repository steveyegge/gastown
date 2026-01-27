# Dolt Commit and Push Workflow

## Quick Reference

```bash
# 1. Check what changed
bd vc status                    # or: cd .beads/dolt/beads && dolt status
bd vc diff --stat              # or: dolt diff --stat

# 2. Stage changes
cd /path/to/.beads/dolt/beads
dolt add .                     # Stage all changes

# 3. Commit
dolt commit -m "Your commit message"

# 4. Push to remote (if configured)
dolt push origin main
```

## Detailed Workflow

### 1. Check Status

**Via bd CLI:**
```bash
bd vc status
```

**Via dolt CLI:**
```bash
cd /path/to/.beads/dolt/beads
dolt status
```

Example output:
```
On branch main

Changes not staged for commit:
  modified:         issues
  modified:         dependencies
  modified:         events
```

### 2. Review Changes

**Summary:**
```bash
cd /path/to/.beads/dolt/beads
dolt diff --stat
```

**Detailed diff:**
```bash
dolt diff issues                # Show changes to issues table
dolt diff dependencies          # Show changes to dependencies table
```

**View specific issue history:**
```bash
bd history <issue-id>
```

### 3. Stage Changes

**Stage all changes:**
```bash
cd /path/to/.beads/dolt/beads
dolt add .
```

**Stage specific tables:**
```bash
dolt add issues dependencies events
```

### 4. Commit Changes

**Simple commit:**
```bash
dolt commit -m "Brief description of changes"
```

**Detailed commit with multi-line message:**
```bash
dolt commit -m "$(cat <<'EOF'
feat: Add new feature beads

Changes:
- Created 5 new task beads
- Updated 3 beads to closed status
- Added 2 convoy tracking relationships

Related: rig-abc123
EOF
)"
```

### 5. View Commit History

**Recent commits:**
```bash
dolt log --oneline -n 10
```

**Full log:**
```bash
dolt log
```

**With graph:**
```bash
dolt log --graph --oneline -n 20
```

### 6. Push to Remote (Optional)

Dolt supports pushing to remote repositories (like DoltHub or self-hosted).

**Check remotes:**
```bash
dolt remote -v
```

**Add a remote:**
```bash
dolt remote add origin https://doltremoteapi.dolthub.com/your-org/your-repo
```

**Push:**
```bash
dolt push origin main
```

**Note:** Most Gas Town installations use Dolt locally without a remote. The `.beads/` directory can be committed to git, including the entire `dolt/` directory for backup.

## Common Scenarios

### Scenario 1: Daily Work Commit

```bash
# After creating/updating several beads during the day
cd /path/to/.beads/dolt/beads
dolt status                     # Check what changed
dolt add .                      # Stage everything
dolt commit -m "Daily work: completed tasks rig-abc, rig-def, rig-ghi"
```

### Scenario 2: Before Creating a PR

```bash
# Commit your Dolt changes before pushing gastown code
cd /path/to/.beads/dolt/beads
dolt diff --stat                # Review what changed
dolt add .
dolt commit -m "feat: storage backend improvements

Created beads for tracking storage backend fixes:
- rig-384557: Convoy watcher fix
- rig-9bb08e: Convoy fetcher fix
- rig-508d0d: Backend detection

Related gastown commit: bf429f7e"

# Then push your gastown code changes
cd /path/to/gastown
git add .
git commit -m "fix(storage): eliminate SQLite dependencies"
git push origin main
```

### Scenario 3: Branch for Experimentation

```bash
cd /path/to/.beads/dolt/beads

# Create a branch
dolt branch experiment

# Switch to it
dolt checkout experiment

# Make changes, commit them
bd create -t task "Experimental feature"
dolt add .
dolt commit -m "test: experimental feature"

# Switch back to main
dolt checkout main

# Merge if successful
dolt merge experiment
```

## Understanding Dolt vs Git

| Feature | Git (Code) | Dolt (Data) |
|---------|-----------|-------------|
| **What it versions** | Files | Database tables |
| **Commands** | git status/add/commit | dolt status/add/commit |
| **Location** | Gastown codebase root | `.beads/dolt/beads/` |
| **Tracks** | Code changes | Issue data changes |
| **Commit shows** | File diffs | Row-level diffs |
| **Branch** | Feature branches | Data experiments |

## Best Practices

1. **Commit regularly** - Don't let changes pile up
   ```bash
   # Good: Commit at end of work session
   dolt commit -m "EOD: completed 5 tasks, updated 3 convoys"
   ```

2. **Use descriptive messages** - Reference issue IDs
   ```bash
   # Good:
   dolt commit -m "feat(rig-abc123): implement new feature

   Created 3 child beads, added dependencies, tested workflow"
   ```

3. **Coordinate with code changes** - Reference related commits
   ```bash
   # Dolt commit
   dolt commit -m "Related gastown commit: bf429f7e"

   # Git commit
   git commit -m "Related dolt commit: lvloeskj"
   ```

4. **Use branches for experiments**
   ```bash
   dolt branch experiment
   dolt checkout experiment
   # Try things out
   dolt checkout main  # Return to main if unsuccessful
   ```

5. **Check before committing**
   ```bash
   dolt diff --stat    # See what changed
   bd list --limit 5   # Verify recent beads look correct
   dolt add .
   dolt commit -m "..."
   ```

## Automated Commits (Future Enhancement)

**Issue:** Dolt doesn't auto-commit like SQLite auto-persists.

**Potential Solutions:**
1. Add git hook: `.beads/hooks/post-update` → `dolt add . && dolt commit`
2. Daemon integration: Auto-commit on idle
3. Periodic commits: Cron job or systemd timer

**For now:** Manual commits as part of workflow.

## Troubleshooting

### "database is read only"

**Cause:** Another process holds Dolt lock.

**Solution:**
```bash
# Check for lock file
ls -la .beads/dolt/beads/.dolt/noms/LOCK

# Find process
lsof .beads/dolt/beads/.dolt/noms/LOCK

# Kill if stale, or wait for completion
```

### Uncommitted changes after bd operations

**Normal!** Dolt requires manual commits.

```bash
cd .beads/dolt/beads
dolt status    # Will show changes
dolt add .
dolt commit -m "..."
```

### Lost commit history

Dolt stores full history in `.dolt/` directory. As long as that exists, history is preserved.

```bash
# View all history
dolt log

# Restore to previous commit
dolt checkout <commit-hash>
```

## Related Documentation

- [Storage Backends Guide](./storage-backends.md) - SQLite vs Dolt comparison
- [Dolt Documentation](https://docs.dolthub.com/)
- Audit Report: `docs/reports/rig-eb4531-storage-audit.md`

---

**Last Updated:** 2026-01-19
**Author:** gastown/crew/dolt_doctor

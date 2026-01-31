# Setup Validation Report: gastown

**Generated:** 2026-01-19
**Status:** DEGRADED
**Polecat:** organic

## Summary

Setup validation completed with warnings and some unfixed issues. The system is functional but has degraded health due to database read-only errors preventing some auto-fixes.

## Upstream Sync Status

| Repository | Status | Details |
|------------|--------|---------|
| gastown | ✓ UP TO DATE | 0 commits behind upstream, 228 commits ahead |
| beads | ✓ SYNCED | Merged 2 commits from upstream, pushed to fork |

## gt doctor Summary

| Metric | Count |
|--------|-------|
| Passed | 49 |
| Warnings | 5 |
| Failed | 2 |

### Failures (could not auto-fix)

1. **agent-beads-exist** - 3 agent beads missing
   - bd-beads-crew-dolt_fixer
   - gt-gastown-crew-beads
   - gt-gastown-crew-dolt_fixer
   - Fix failed: `database is read only`

2. **clone-divergence** - 1 clone critically diverged
   - beads/mayor/rig: 51 commits behind origin/main (EMERGENCY)
   - beads/refinery/rig: 48 commits behind origin/main

### Warnings

- sqlite3 CLI not found (needed for convoy features)
- 1 rig missing patrol molecules (cicd)
- 4 stuck patrol wisps found (>1h old)
- 34 orphan processes running outside tmux
- 8 persistent roles not on main branch

## bd doctor Summary

| Metric | Count |
|--------|-------|
| Passed | 53 |
| Warnings | 11 |
| Failed | 1 |

### Failures

1. **Sync Branch Config** - Currently on sync branch 'master'
   - bd sync cannot create worktree for checked-out branch

### Warnings

- Multiple JSONL files (issues.jsonl, routes.jsonl)
- Config routing.mode value invalid
- DB-JSONL sync mismatch (1175 DB vs 921 JSONL)
- Sync divergence detected
- Uncommitted changes present
- Claude plugin not installed
- 4 stale molecules
- 111 orphaned dependencies
- 43 duplicate issues
- 1 test pollution issue

## Manual Fixes Required

### High Priority

1. **Clone divergence (EMERGENCY)**
   ```bash
   cd /home/ubuntu/pihealth/beads/mayor/rig
   git pull --rebase
   cd /home/ubuntu/pihealth/beads/refinery/rig
   git pull --rebase
   ```

2. **Database read-only issue** - Investigate why bd commands are hitting read-only database

### Medium Priority

3. **Install sqlite3**
   ```bash
   sudo apt install sqlite3
   ```

4. **DB-JSONL sync**
   ```bash
   bd export
   ```

5. **Clean up orphaned dependencies**
   ```bash
   bd doctor --fix  # When database is writable
   ```

### Low Priority

6. Review and close 4 stale molecules: `bd mol stale`
7. Review 43 duplicate issues: `bd duplicates`
8. Install Claude plugin for better integration

## Recommendations

1. **Immediate:** Fix the clone divergence - beads/mayor/rig is 51 commits behind which is flagged as EMERGENCY
2. **Short-term:** Investigate the database read-only errors blocking auto-fixes
3. **Ongoing:** Install sqlite3 for full convoy feature support

## Tools Rebuilt

- gt: v0.2.6-403-gee9a6aa5
- bd: v0.48.0

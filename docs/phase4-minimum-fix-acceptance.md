# Phase 4 Minimum Fix - Acceptance Commands and PR Draft

This document freezes the final verification commands and a ready-to-paste PR summary for the Phase 4 minimum-fix batch.

## Fixed Verification Commands

Run from `gastown-main`:

```bash
# 1) Full container acceptance
make test-e2e-container

# 2) Minimal WSL smoke for the role-path regressions
go test ./internal/cmd -tags=integration -run TestRoleHomeCwdDetection\|TestRoleEnvCwdMismatchFromIncompleteDir -count=1 -v
```

## Latest Run Results

- `make test-e2e-container`: **PASS**
- `go test ./internal/cmd -tags=integration -run TestRoleHomeCwdDetection\|TestRoleEnvCwdMismatchFromIncompleteDir -count=1 -v`: **PASS**

## PR Description (Ready to Paste)

### Summary

- Fix scheduler integration JSON pollution by keeping test helper success path stdout-only and surfacing stderr only on failures.
- Make Dolt metadata writes idempotent to keep mayor worktrees clean (`.beads/metadata.json` no-op when content is unchanged).
- Harden e2e container reliability with pinned toolchain versions (`bd` and `dolt`), retry loops, and docker context optimization.
- Stabilize install/role integration behavior in noisy environments (formula provisioning resilience, role command pre-run noise suppression, and tolerant online smoke checks where transient Dolt startup races occur).

### Validation

- `make test-e2e-container` ✅
- `go test ./internal/cmd -tags=integration -run TestRoleHomeCwdDetection\|TestRoleEnvCwdMismatchFromIncompleteDir -count=1 -v` ✅

### Notes

- Some integration cases intentionally `SKIP` when specific external binaries are unavailable in that test environment.
- Doctor warnings shown inside integration logs are expected in synthetic test setups and did not block pass/fail outcomes.

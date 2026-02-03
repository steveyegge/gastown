# Local Build State

Note: When applying PRs from forks, cherry-pick the specific commits (`gh pr view <PR#> --json commits`) rather than merging the branch, which may include unrelated commits.

## Base
upstream/main: e2116bbd (v0.5.0+184)

## Applied PRs

| PR | Commit | Description |
|----|--------|-------------|
| #528 | 11051b3d | fix(seance): run claude --resume from session's original cwd |
| #530 | 25c6e2ba | docs: Add PR branch naming guidance to CONTRIBUTING.md |
| #608 | 3669c04f | fix(doctor): add allowed_prefixes check for convoy beads |
| #799 | e32c5f26 | feat(cmd): add preflight and postflight workspace commands |
| #989 | d9b86dfa | perf(status): parallelize beads pre-fetching (~2x faster) |
| #994 | 5d2914a4 | fix(doctor): update claude_settings_check to use working directories |
| #1042 | 0d9c81dd | feat(doctor): distinguish fixed vs unfixed issues in --fix output |
| #1092 | 7df00d85 | feat(deacon): add feed-stranded-convoys step to patrol (PiotrTrzpil) |
| #1106 | e7336993 | fix(patrol): use gt convoy commands instead of bd list |
| #1111 | 0f56470e | feat(tmux): add C-b g keybinding for agent switcher menu (groblegark) |
| #1113 | e480fa19 | fix(daemon): prevent deadlock in errant beads migration |
| #1117 | 3021521f | fix(daemon): check flock status instead of file existence |
| #1118 | 6b2e3311 | fix(convoy): detect orphaned molecules as stranded |
| #1119 | 65d96b07 | fix(deacon): update heartbeat on every startup, including resume |
| #1122 | b79038f3 | fix(polecat): prevent orphaned hooked work during concurrent sling |
| #1123 | b0f98bae | fix(boot): use flock instead of session check in AcquireLock |
| #1125 | 0c2b32eb | fix(boot): add idle detection to triage |
| #1126 | 006cf263 | fix(boot): ensure Boot is ephemeral (fresh each tick) |
| #1127 | f8b01d83 | feat: add gt dog clear and gt warrant commands |
| #1161 | ef45859a | fix(dashboard): use townRoot for bd commands in cross-rig routing |
| #1168 | 450a661d | fix(refinery): nudge refinery when MR is created, not at sling time (dcosson) |
| #1172 | 6759181d | fix(done): retry molecule close with exponential backoff |
| - | bbf0aa07 | fix: resolve interface mismatches from cherry-picks |

## Skipped PRs

| PR | Reason |
|----|--------|
| #734 | File deleted upstream (stale_beads_redirect_check.go) - needs rebase |

## Build from Source (Linux)

Building requires CGO and these system dependencies:

```bash
# Debian/Ubuntu
sudo apt-get install -y gcc g++ libzstd-dev libicu-dev

# Build and install (ldflags required for version info and BuiltProperly check)
VERSION=$(git describe --tags --always --dirty | sed 's/^v//') && \
COMMIT=$(git rev-parse --short HEAD) && \
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ") && \
go generate ./... && \
go install -ldflags "-X github.com/steveyegge/gastown/internal/cmd.Version=$VERSION -X github.com/steveyegge/gastown/internal/cmd.Commit=$COMMIT -X github.com/steveyegge/gastown/internal/cmd.BuildTime=$BUILD_TIME -X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1" ./cmd/gt

# After pushing to origin/main, update the mayor's rig clone to avoid stale binary warnings:
git -C ~/gt/gastown/mayor/rig pull
```

**Important:** Build `gt` only after ALL commits (including this file's updates) are pushed.
The stale binary warning compares against `~/gt/gastown/mayor/rig` (origin/main). If you build
before pushing UPDATE-NOTES.md changes, the binary will appear stale.

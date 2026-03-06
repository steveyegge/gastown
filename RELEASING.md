# Releasing Gas Town

## Distribution Channels

| Channel | Mechanism | Automatic? |
|---------|-----------|------------|
| **GitHub Release** | GoReleaser via Actions on tag push | Yes |
| **Homebrew** (homebrew-core) | Homebrew bot detects new release | Yes (24-48h delay) |
| **npm** (`@gastown/gt`) | Actions workflow, OIDC trusted publishing | Yes (when org is set up) |

## How to Release

### Option A: Automated (recommended)

Use the release formula, which handles all steps:

```bash
gt mol wisp create gastown-release --var version=X.Y.Z
```

### Option B: Bump script

```bash
cd gastown/mayor/rig
./scripts/bump-version.sh X.Y.Z --commit --tag --push --install
```

### Option C: Manual

1. Update CHANGELOG.md `[Unreleased]` section
2. Update `internal/cmd/info.go` `versionChanges` slice
3. Run `./scripts/bump-version.sh X.Y.Z` (updates version.go, package.json, CHANGELOG header)
4. Commit, tag, push:

```bash
git add -A
git commit -m "chore: Bump version to X.Y.Z"
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin main
git push origin vX.Y.Z
```

5. Rebuild locally:

```bash
make install        # builds, codesigns, installs to ~/.local/bin
gt daemon stop && gt daemon start
```

## What Happens After Tag Push

The `release.yml` workflow triggers automatically:

1. **goreleaser** job builds binaries for all platforms and creates the GitHub Release
2. **publish-npm** job publishes to npm (best-effort, `continue-on-error: true`)

Homebrew is NOT updated by the workflow. See below.

## Homebrew (homebrew-core)

Gastown is in **homebrew-core** (not a custom tap). The formula lives at:
`https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/g/gastown.rb`

### How it updates

Homebrew's `BrewTestBot` automatically detects new GitHub releases and opens
a PR to homebrew-core. Gastown is on the autobump list — the bot checks
**every ~3 hours**.

### If the bot doesn't pick it up

Gastown is on the autobump list, so `brew bump-formula-pr` will refuse to
submit a manual PR. If the bot hasn't updated after 6+ hours, check
https://github.com/Homebrew/homebrew-core/pulls?q=gastown for stuck PRs.

### Verifying

```bash
brew update
brew info gastown    # Check version
brew upgrade gastown # Upgrade if installed
```

## npm (`@gastown/gt`)

### How it works

The workflow uses **OIDC trusted publishing** (npm provenance). No NPM_TOKEN
secret is needed — the `id-token: write` permission on the job generates a
short-lived OIDC token that npm trusts because the GitHub repo is linked to
the npm package.

### Prerequisites

The `@gastown` npm organization must exist and be linked to this repo:

1. Go to https://www.npmjs.com and create (or join) the `@gastown` org
2. Under org settings, enable "Require 2FA" and configure trusted publishing
3. Link `steveyegge/gastown` as a trusted publisher for `@gastown/gt`

### Current status (as of 2026-03-06)

The `@gastown` npm org was secured by a community member (Ivan Casco Valero,
ivan@ivancasco.com) to prevent scope squatting. Ownership transfer is pending.
Until the org is transferred, npm publish will fail gracefully without blocking
the release (`continue-on-error: true` in the workflow).

### Verifying

```bash
npm view @gastown/gt version
npm install -g @gastown/gt
gt version
```

## Files Updated During Release

| File | What changes |
|------|-------------|
| `CHANGELOG.md` | New version section with date |
| `internal/cmd/info.go` | `versionChanges` entry for `gt info --whats-new` |
| `internal/cmd/version.go` | `Version` constant |
| `npm-package/package.json` | `version` field |
| `flake.nix` | version + vendorHash (only if `nix` is in PATH) |

## Troubleshooting

### GoReleaser fails with "replace directives"

The workflow rejects `go.mod` files with `replace` directives (they break
`go install`). Remove the replace directive and commit before tagging.

### npm publish returns 404

The `@gastown` npm org doesn't exist or you don't have publish access.
See the npm section above. The release still succeeds — npm is best-effort.

### Homebrew shows old version after 6+ hours

Gastown is on BrewTestBot's autobump list (checked every ~3h). Check
https://github.com/Homebrew/homebrew-core/pulls?q=gastown for stuck PRs.
Manual `brew bump-formula-pr` is blocked for autobump formulae.

### `make install` shows `-dirty` suffix

The `.beads/` directory has unstaged changes. This is cosmetic — the version
number is correct. The `-dirty` comes from `git describe` seeing any unstaged
modifications.

### Version in version.go is still old after bump script

The bump script reads the current version from version.go and replaces it.
If version.go was manually edited to a different version, the script's sed
pattern won't match. Fix version.go manually and re-run.

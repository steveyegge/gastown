# Releasing Gas Town

## Channel Policy

When Gas Town docs or release notes say "latest community release", they mean
the newest GitHub release tag whose `Release` workflow completed successfully.
Homebrew and npm are downstream distribution channels, not the source of truth.

| Channel | Role | Authoritative for `latest`? | Expected delay | What to verify |
|---------|------|------------------------------|----------------|----------------|
| **GitHub Releases** | Canonical published release record and binary artifacts | **Yes** | Immediate once the tag workflow succeeds | Release page exists, artifacts are attached, checksums are present |
| **Homebrew** (`homebrew-core`) | Primary package-manager channel for end users on macOS/Linux | No | Bot-driven and can lag for several hours; allow 24-48h before treating it as stuck | `brew info gastown` eventually reports the new version |
| **npm** (`@gastown/gt`) | Convenience wrapper that downloads GitHub release binaries | No | Best-effort and may lag or be temporarily unavailable | `npm view @gastown/gt version` may catch up later |
| **Source checkout** (`make install`) | Developer/local build path | No | N/A | Your local binary matches the checked-out commit, not the newest published release |

Interpretation rules:

- Verify GitHub Releases first. If GitHub is correct, the release exists.
- Homebrew lag does not mean the release failed.
- npm lag or publish failure does not block the GitHub release.
- Do not describe npm as the authoritative "latest" version; it mirrors GitHub
  artifacts only after `publish-npm` succeeds.

## How to Release

### Option A: Automated (recommended)

Use the release formula, which walks the release owner through the same policy
documented here:

```bash
gt mol wisp create gastown-release --var version=X.Y.Z
```

### Option B: Bump script

```bash
cd gastown/mayor/rig
./scripts/bump-version.sh X.Y.Z --commit --tag --push --install
```

### Option C: Manual

1. Update `CHANGELOG.md` `[Unreleased]`
2. Update `internal/cmd/info.go` `versionChanges`
3. Run `./scripts/bump-version.sh X.Y.Z`
4. Commit, tag, and push:

```bash
git add -A
git commit -m "chore: Bump version to X.Y.Z"
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin main
git push origin vX.Y.Z
```

5. Refresh your local source install:

```bash
make install
gt version
```

`make install` is the canonical source/dev install path. It installs to
`~/.local/bin/gt`, removes stale shadowing copies from `~/go/bin` and
`~/bin`, and restarts the daemon if one is already running.

## What Happens After Tag Push

The `Release` workflow triggers automatically:

1. `goreleaser` builds the release artifacts and creates the GitHub Release.
2. `publish-npm` attempts to publish `@gastown/gt` with trusted publishing.
3. Homebrew updates later via `homebrew-core` autobump after the GitHub
   release exists.

The release should be considered complete when the GitHub Release is present
and the release workflow is green. Homebrew and npm are follow-up verification
steps, not the gating definition of whether `vX.Y.Z` exists.

## Post-release Verification

### GitHub Release

Verify this first:

```bash
gh release view vX.Y.Z --repo steveyegge/gastown
```

Or inspect:
`https://github.com/steveyegge/gastown/releases/tag/vX.Y.Z`

Expected:

- Release page exists
- Archives for supported platforms are attached
- Checksums are present

### Homebrew (`homebrew-core`)

Gastown is published from `homebrew-core`:
`https://github.com/Homebrew/homebrew-core/blob/HEAD/Formula/g/gastown.rb`

Homebrew's autobump bot polls for new upstream releases. It is normal for
Homebrew to remain behind GitHub for a while after a tag lands.

Verify with:

```bash
brew update
brew info gastown
```

If Homebrew has not updated after roughly 24-48 hours, inspect open PRs:
`https://github.com/Homebrew/homebrew-core/pulls?q=gastown`

### npm (`@gastown/gt`)

The npm package is a thin launcher that downloads the platform-specific GitHub
release artifact matching the npm package version. npm is useful for users who
prefer `npm install -g`, but it is not the release authority.

The workflow uses OIDC trusted publishing. The `@gastown` npm organization must
be configured to trust `steveyegge/gastown` as a publisher.

Verify with:

```bash
npm view @gastown/gt version
npm install -g @gastown/gt
gt version
```

If npm still reports an older version while GitHub Releases is correct, treat
that as a downstream packaging follow-up. Do not re-tag just to "fix npm".

## Files Updated During Release

| File | What changes |
|------|--------------|
| `CHANGELOG.md` | New version section with date |
| `internal/cmd/info.go` | `versionChanges` entry for `gt info --whats-new` |
| `internal/cmd/version.go` | `Version` constant |
| `npm-package/package.json` | npm package version |
| `flake.nix` | Version + `vendorHash` (only if `nix` is in `PATH`) |

## Troubleshooting

### GoReleaser fails with "replace directives"

The workflow rejects `go.mod` files with `replace` directives because they
break `go install ...@latest`. Remove the replace directive and retag only
after the branch is clean.

### npm publish fails or returns 404

Trusted publishing is not configured correctly, or the package/org permissions
are incomplete. The GitHub release still stands; fix npm publishing separately
and republish the package if needed.

### Homebrew still shows the previous version

Homebrew lag is expected immediately after a GitHub release. Only treat it as
stuck after the autobump window has passed, then inspect homebrew-core PRs.

### `make install` shows a `-dirty` suffix

The working tree has unstaged changes. The semantic version is still correct;
the suffix comes from `git describe`.

### `version.go` did not update

`scripts/bump-version.sh` replaces the current literal version. If the file was
edited manually into an unexpected shape, fix `internal/cmd/version.go` and rerun
the bump script.

# Release Process for Gas Town

This document describes the release process for Gas Town, including GitHub releases and npm packages.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Release Checklist](#release-checklist)
- [1. Prepare Release](#1-prepare-release)
- [2. GitHub Release](#2-github-release)
- [3. npm Package Release](#3-npm-package-release)
- [4. Verify Release](#4-verify-release)
- [Hotfix Releases](#hotfix-releases)

## Overview

A Gas Town release involves multiple distribution channels:

1. **GitHub Release** - Binary downloads for all platforms
2. **npm** - Node.js package for cross-platform installation (`@gastown/gt`)

## Prerequisites

### Required Tools

- `git` with push access to steveyegge/gastown
- `goreleaser` for building binaries
- `npm` with authentication (for npm releases)
- `gh` CLI (GitHub CLI, recommended)

### Required Access

- GitHub: Write access to repository and ability to create releases
- npm: Publish access to `@gastown` organization

### Verify Setup

```bash
# Check git
git remote -v  # Should show steveyegge/gastown

# Check goreleaser
goreleaser --version

# Check GitHub CLI
gh auth status

# Check npm
npm whoami  # Should show your npm username
```

## Release Checklist

Before starting a release:

- [ ] All tests passing (`go test ./...`)
- [ ] npm package tests passing (`cd npm-package && npm test`)
- [ ] CHANGELOG.md updated with release notes
- [ ] No uncommitted changes
- [ ] On `main` branch and up to date with origin

## 1. Prepare Release

### Update CHANGELOG.md

Add release notes to CHANGELOG.md following the Keep a Changelog format:

```markdown
## [0.2.0] - 2026-01-15

### Added
- New feature X

### Changed
- Improved Y

### Fixed
- Bug in Z
```

Commit the CHANGELOG changes:

```bash
git add CHANGELOG.md
git commit -m "docs: Add CHANGELOG entry for v0.2.0"
git push origin main
```

### Update Version

Update version in relevant files:

1. `internal/cmd/version.go` - CLI version constant
2. `npm-package/package.json` - npm package version

```bash
# Update versions
vim internal/cmd/version.go
vim npm-package/package.json

# Commit
git add -A
git commit -m "chore: Bump version to 0.2.0"
git push origin main
```

### Create Release Tag

```bash
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

This triggers GitHub Actions to build release artifacts automatically.

## 2. GitHub Release

### Using GoReleaser (Recommended)

GoReleaser automates binary building and GitHub release creation:

```bash
# Clean any previous builds
rm -rf dist/

# Create release (uses GITHUB_TOKEN from gh CLI)
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean
```

This will:
- Build binaries for all platforms (macOS, Linux, Windows - amd64/arm64)
- Create checksums
- Generate release notes from CHANGELOG.md
- Upload everything to GitHub releases

### Verify GitHub Release

1. Visit https://github.com/steveyegge/gastown/releases
2. Verify the new version is marked as "Latest"
3. Check all platform binaries are present

## 3. npm Package Release

The npm package wraps the native binary for Node.js environments.

### Test Installation Locally

```bash
cd npm-package

# Run tests
npm test

# Pack and test install
npm pack
npm install -g ./gastown-gt-0.2.0.tgz
gt version  # Should show 0.2.0

# Cleanup
npm uninstall -g @gastown/gt
rm gastown-gt-0.2.0.tgz
```

### Publish to npm

```bash
# IMPORTANT: Ensure GitHub release with binaries is live first!
cd npm-package
npm publish --access public
```

### Verify npm Release

```bash
npm install -g @gastown/gt
gt version  # Should show 0.2.0
```

## 4. Verify Release

After all channels are updated:

### GitHub

```bash
# Download and test binary
curl -LO https://github.com/steveyegge/gastown/releases/download/v0.2.0/gastown_0.2.0_darwin_arm64.tar.gz
tar -xzf gastown_0.2.0_darwin_arm64.tar.gz
./gt version
```

### npm

```bash
npm install -g @gastown/gt
gt version
```

## Hotfix Releases

For urgent bug fixes:

```bash
# Create hotfix branch from tag
git checkout -b hotfix/v0.2.1 v0.2.0

# Make fixes and bump version
# ... edit files ...

# Commit, tag, and release
git add -A
git commit -m "fix: Critical bug fix"
git tag -a v0.2.1 -m "Hotfix release v0.2.1"
git push origin hotfix/v0.2.1
git push origin v0.2.1

# Follow normal release process
GITHUB_TOKEN=$(gh auth token) goreleaser release --clean

# Merge back to main
git checkout main
git merge hotfix/v0.2.1
git push origin main
```

## Version Numbering

Gas Town follows [Semantic Versioning](https://semver.org/):

- **MAJOR** (x.0.0): Breaking changes
- **MINOR** (0.x.0): New features, backwards compatible
- **PATCH** (0.0.x): Bug fixes, backwards compatible

## Questions?

- Open an issue: https://github.com/steveyegge/gastown/issues
- Check existing releases: https://github.com/steveyegge/gastown/releases

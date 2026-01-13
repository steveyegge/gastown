# Upstream PR Protocol

This document describes the protocol for contributing pull requests upstream to the original Gas Town repository maintained by Steve Yegge.

## Upstream Repository

- **URL**: https://github.com/steveyegge/gastown
- **Primary Branch**: `main`
- **License**: MIT

## Fork Relationship

Our fork (`groblegark/gastown`) tracks upstream. The typical flow:

```
steveyegge/gastown (upstream)
       ↓
groblegark/gastown (origin)
       ↓
local worktree (your work)
```

## Contribution Workflow

### 1. Ensure Your Fork is Current

```bash
# Add upstream if not already configured
git remote add upstream https://github.com/steveyegge/gastown.git

# Fetch and merge upstream changes
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

### 2. Create a Feature Branch

```bash
git checkout -b feat/your-feature-name main
```

### 3. Make Your Changes

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Add comments for non-obvious logic
- Include tests for new functionality

### 4. Run Quality Checks

```bash
# Build
go build -o gt ./cmd/gt

# Run tests
go test ./...

# Run linter (if golangci-lint installed)
golangci-lint run
```

The project uses these linters:
- **errcheck**: Detects unchecked error returns
- **gosec**: Identifies security vulnerabilities
- **misspell**: Finds spelling errors (US locale)
- **unconvert**: Removes unnecessary type conversions
- **unparam**: Detects unused function parameters

### 5. Commit Your Changes

Follow commit message conventions:
- Use present tense ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable: `Fix timeout bug (gt-xxx)`

Example:
```bash
git commit -m "feat(convoy): add close command for manual convoy closure"
```

### 6. Push and Create PR

```bash
git push origin feat/your-feature-name
```

Then create a pull request via GitHub from `groblegark/gastown:feat/your-feature-name` to `steveyegge/gastown:main`.

## PR Guidelines

### What Makes a Good PR

- **Bug fixes** with clear reproduction steps
- **Documentation improvements**
- **Test coverage expansion**
- **Small, focused features**

### For Larger Changes

Open an issue first to discuss the approach before investing significant effort.

### PR Description Template

```markdown
## Summary
Brief description of what this PR does.

## Changes
- List of specific changes

## Testing
How this was tested.

## Related Issues
Fixes #123 (if applicable)
```

## Licensing Requirements

The project uses the **MIT License**. There is no CLA (Contributor License Agreement).

By submitting a PR, you agree that your contributions will be licensed under the same MIT license.

### Attribution

The MIT license requires:
1. Include the copyright notice: "Copyright (c) 2025 Steve Yegge"
2. Retain the license text in distributions

## Code Style

### Go Conventions
- Use `gofmt` for formatting
- Run `go vet` for static analysis
- Follow idiomatic Go patterns

### Project-Specific
- Main CLI code lives in `cmd/gt/`
- Core logic lives in `internal/`
- Documentation in `docs/`
- Shell integration in `scripts/`

## Syncing Upstream Changes

To pull in upstream changes to our fork:

```bash
git fetch upstream
git checkout main
git merge upstream/main
git push origin main
```

## Quick Reference

| Item | Value |
|------|-------|
| Upstream URL | https://github.com/steveyegge/gastown |
| Primary Branch | `main` |
| License | MIT (no CLA) |
| Test Command | `go test ./...` |
| Build Command | `go build -o gt ./cmd/gt` |
| Lint Tool | golangci-lint |

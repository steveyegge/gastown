# Contributing to Gas Town

Thanks for your interest in contributing! Gas Town is experimental software, and we welcome contributions that help explore these ideas.

## Getting Started

1. Fork the repository
2. Clone your fork
3. Install prerequisites (see README.md)
4. Build and test: `go build -o gt ./cmd/gt && go test ./...`

## Development Workflow

We use a direct-to-main workflow for trusted contributors. For external contributors:

1. Create a feature branch from `main`
2. Make your changes
3. Ensure tests pass: `go test ./...`
4. Submit a pull request

### PR Branch Naming

**Never create PRs from your fork's `main` branch.** Always create a dedicated branch for each PR:

```bash
# Good - dedicated branch per PR
git checkout -b fix/deacon-startup upstream/main
git checkout -b feat/auto-seance upstream/main

# Bad - PR from main accumulates unrelated commits
git checkout main  # Don't PR from here!
```

Why this matters:
- PRs from `main` accumulate ALL commits pushed to your fork
- Multiple contributors pushing to the same fork's `main` creates chaos
- Reviewers can't tell which commits belong to which PR
- You can't have multiple PRs open simultaneously

Branch naming conventions:
- `fix/*` - Bug fixes
- `feat/*` - New features
- `refactor/*` - Code restructuring
- `docs/*` - Documentation only

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions focused and small
- Add comments for non-obvious logic
- Include tests for new functionality

## What to Contribute

Good first contributions:
- Bug fixes with clear reproduction steps
- Documentation improvements
- Test coverage for untested code paths
- Small, focused features

For larger changes, please open an issue first to discuss the approach.

## Commit Messages

- Use present tense ("Add feature" not "Added feature")
- Keep the first line under 72 characters
- Reference issues when applicable: `Fix timeout bug (gt-xxx)`

## Testing

Run the full test suite before submitting:

```bash
go test ./...
```

For specific packages:

```bash
go test ./internal/wisp/...
go test ./cmd/gt/...
```

## Questions?

Open an issue for questions about contributing. We're happy to help!

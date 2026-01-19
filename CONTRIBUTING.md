# Contributing to Gas Town

Thanks for your interest in contributing! Gas Town is experimental software, and we welcome contributions that help explore these ideas.

## Getting Started

1. Fork the repository
2. Clone your fork
3. Install prerequisites (see README.md)
4. Build and test: `go build -o gt ./cmd/gt && go test ./...`

## Before Making Changes

**Always check for existing PRs first.** Many issues have already been addressed:

```bash
# Check open PRs for related fixes
gh pr list --repo steveyegge/gastown --state open

# Or with curl
curl -s "https://api.github.com/repos/steveyegge/gastown/pulls?state=open" | \
  jq -r '.[] | "#\(.number): \(.title)"'
```

If a relevant PR exists:
1. Review it for completeness
2. Comment if you find gaps or have suggestions
3. Only create a new PR if your fix is complementary (different scope)

## Development Workflow

We use a direct-to-main workflow for trusted contributors. For external contributors:

1. Check open PRs first (see above)
2. Create a feature branch from `main`
3. Make your changes
4. Ensure tests pass: `go test ./...`
5. Submit a pull request

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

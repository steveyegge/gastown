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

If you want to verify in a clean container (with the same steps used in CI), run:

```bash
make container-test
```

For specific packages:

```bash
go test ./internal/wisp/...
go test ./cmd/gt/...
```

## Questions?

Open an issue for questions about contributing. We're happy to help!

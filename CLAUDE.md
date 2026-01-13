# Claude Code Instructions

This file contains instructions and standards for AI agents working on this codebase.

## Coding Standards

Follow these conventions for all Go code in this repository:

1. **Go conventions**: Run `gofmt` and `go vet` - code must pass both
2. **Small functions**: Keep functions focused and small (single responsibility)
3. **Comments**: Add comments for non-obvious logic
4. **Tests**: Include tests for new functionality

## Commit Messages

Follow these conventions for all commits:

1. **Present tense**: Use "Add feature" not "Added feature"
2. **Short subject**: Keep the first line under 72 characters
3. **Reference issues**: Include bead/issue IDs when applicable, e.g. `Fix timeout bug (gt-xxx)`

## Pull Requests

1. **No bead IDs in PRs**: Do NOT include bead IDs (gt-xxx) anywhere in PR descriptions - not in Closes, Ref, or anywhere else. Bead IDs are internal tracking only.
2. **GitHub issues only**: Use 'Fixes #xxx' only for actual GitHub issue numbers

## AI-First Philosophy

Gas Town is designed for AI agents. When making improvements:

**Prioritize:**
- Machine-parseable output (JSON flags, structured errors)
- Predictable APIs and consistent patterns AI can learn
- Clear, actionable error messages for programmatic handling
- Automation hooks and extension points
- State introspection capabilities
- Reducing ambiguity in command behavior

**Deprioritize:**
- Human-readable formatting and colors
- Interactive prompts
- Documentation prose
- CLI UX polish for humans

## Iteration Workflow

Before creating a PR, iterate 5 times on your implementation. Each iteration must be a demonstrable IMPROVEMENT over the previous one.

**Iteration Requirements:**
1. Each iteration must make the code measurably better
2. Don't just re-run tests - actually improve something
3. Build and test must pass after each iteration

**Types of Improvements:**
- **Iter 1**: Basic implementation that works
- **Iter 2**: Better error handling, edge cases, cleaner code
- **Iter 3**: More edge cases, better tests, documentation
- **Iter 4**: Refactor for clarity, optimize hot paths
- **Iter 5**: Final polish, perfect tests, audit for issues

**After Each Iteration:**
1. Run `go build ./...` - must pass
2. Run `go vet ./...` - must pass
3. Run tests for affected packages - must pass
4. Briefly note what improved

**Only after all 5 iterations pass** should you create the PR.

After PR creation, wait 2 minutes then check `gh pr checks <number>`. Fix any failures until all pass.

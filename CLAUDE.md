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

Before creating a PR, complete 5 improved iterations:

1. **Iter 1**: Get basic functionality working
2. **Iter 2**: IMPROVE - better error handling, cleaner code
3. **Iter 3**: IMPROVE - more edge cases, better tests
4. **Iter 4**: IMPROVE - refactor for clarity, optimize
5. **Iter 5**: IMPROVE - final polish, perfect tests

Each iteration must be demonstrably better than the last. Don't just review - actively improve.

After PR creation, wait 2 minutes then check `gh pr checks <number>`. Fix any failures until all pass.

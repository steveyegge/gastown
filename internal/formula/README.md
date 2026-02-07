# Formula Package

TOML-based workflow definitions with validation, cycle detection, and execution planning.

## Overview

The formula package parses and validates structured workflow definitions, enabling:

- **Type inference** - Automatically detect formula type from content
- **Validation** - Check required fields, unique IDs, valid references
- **Cycle detection** - Prevent circular dependencies
- **Topological sorting** - Compute dependency-ordered execution
- **Ready computation** - Find steps with satisfied dependencies

## Installation

```go
import "github.com/steveyegge/gastown/internal/formula"
```

## Quick Start

```go
// Parse a formula file
f, err := formula.ParseFile("workflow.formula.toml")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Formula: %s (type: %s)\n", f.Name, f.Type)

// Get execution order
order, _ := f.TopologicalSort()
fmt.Printf("Execution order: %v\n", order)

// Track and execute
completed := make(map[string]bool)
for len(completed) < len(order) {
    ready := f.ReadySteps(completed)
    // Execute ready steps (can be parallel)
    for _, id := range ready {
        step := f.GetStep(id)
        fmt.Printf("Executing: %s\n", step.Title)
        completed[id] = true
    }
}
```

## Formula Types

### Workflow

Sequential steps with explicit dependencies. Steps execute when all `needs` are satisfied.

```toml
formula = "release"
description = "Standard release process"
type = "workflow"

[vars.version]
description = "Version to release"
required = true

[[steps]]
id = "test"
title = "Run Tests"
description = "Execute test suite"

[[steps]]
id = "build"
title = "Build Artifacts"
needs = ["test"]

[[steps]]
id = "publish"
title = "Publish Release"
needs = ["build"]
```

### Convoy

Parallel legs that execute independently, with optional synthesis.

```toml
formula = "security-scan"
type = "convoy"

[[legs]]
id = "sast"
title = "Static Analysis"
focus = "Code vulnerabilities"

[[legs]]
id = "deps"
title = "Dependency Audit"
focus = "Vulnerable packages"

[[legs]]
id = "secrets"
title = "Secret Detection"
focus = "Leaked credentials"

[synthesis]
title = "Security Report"
description = "Combine all findings"
depends_on = ["sast", "deps", "secrets"]
```

### Expansion

Template-based formulas for parameterized workflows.

```toml
formula = "component-review"
type = "expansion"

[[template]]
id = "analyze"
title = "Analyze {{component}}"

[[template]]
id = "test"
title = "Test {{component}}"
needs = ["analyze"]
```

### Aspect

Multi-aspect parallel analysis (similar to convoy).

```toml
formula = "code-review"
type = "aspect"

[[aspects]]
id = "security"
title = "Security Review"
focus = "OWASP Top 10"

[[aspects]]
id = "performance"
title = "Performance Review"
focus = "Complexity and bottlenecks"

[[aspects]]
id = "maintainability"
title = "Maintainability Review"
focus = "Code clarity and documentation"
```

## API Reference

### Parsing

```go
// Parse from file
f, err := formula.ParseFile("path/to/formula.toml")

// Parse from bytes
f, err := formula.Parse([]byte(tomlContent))
```

### Validation

Validation is automatic during parsing. Errors are descriptive:

```go
f, err := formula.Parse(data)
// Possible errors:
// - "formula field is required"
// - "invalid formula type \"foo\""
// - "duplicate step id: build"
// - "step \"deploy\" needs unknown step: missing"
// - "cycle detected involving step: a"
```

### Execution Planning

```go
// Get dependency-sorted order
order, err := f.TopologicalSort()

// Find ready steps given completed set
completed := map[string]bool{"test": true, "lint": true}
ready := f.ReadySteps(completed)

// Lookup individual items
step := f.GetStep("build")
leg := f.GetLeg("sast")
tmpl := f.GetTemplate("analyze")
aspect := f.GetAspect("security")
```

### Dependency Queries

```go
// Get all item IDs
ids := f.GetAllIDs()

// Get dependencies for a specific item
deps := f.GetDependencies("build")  // Returns ["test"]
```

## Embedded Formulas

The package embeds 30+ common formulas for Gas Town workflows from `internal/formula/formulas/` (the single source of truth). Formulas are compiled into the `gt` binary and resolved at runtime: rig → town → embedded.

```go
// Read an embedded formula by name
content, err := formula.GetEmbeddedFormula("shiny")

// List all embedded formula names
names, err := formula.GetEmbeddedFormulaNames()

// Check if an embedded formula exists
exists := formula.EmbeddedFormulaExists("shiny")

// Get SHA-256 hash of an embedded formula
hash, err := formula.GetEmbeddedFormulaHash("shiny")

// Copy an embedded formula to a local path (for gt formula modify)
path, err := formula.CopyFormulaTo("shiny", "/path/to/.beads/formulas/")

// Extract base version hash from an override file's header comment
baseHash := formula.ExtractBaseHash(overrideContent)
```

## Testing

```bash
go test ./internal/formula/... -v
```

The package has 130% test coverage (1,200 lines of tests for 925 lines of code).

## Dependencies

- `github.com/BurntSushi/toml` - TOML parsing (stable, widely-used)

## License

MIT License - see repository LICENSE file.

// Package formula provides parsing, validation, and execution planning for
// TOML-based workflow definitions.
//
// # Overview
//
// The formula package enables structured workflow definitions with dependency
// tracking, validation, and parallel execution planning. It supports four
// formula types, each designed for different execution patterns:
//
//   - convoy: Parallel execution of independent legs with synthesis
//   - workflow: Sequential steps with explicit dependencies
//   - expansion: Template-based step generation
//   - aspect: Multi-aspect parallel analysis
//
// # Quick Start
//
// Parse a formula file and get execution order:
//
//	f, err := formula.ParseFile("workflow.formula.toml")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Get topologically sorted execution order
//	order, err := f.TopologicalSort()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Execute steps, tracking completion
//	completed := make(map[string]bool)
//	for len(completed) < len(order) {
//	    ready := f.ReadySteps(completed)
//	    // Execute ready steps in parallel...
//	    for _, id := range ready {
//	        completed[id] = true
//	    }
//	}
//
// # Formula Types
//
// Convoy formulas execute legs in parallel, then synthesize results:
//
//	formula = "security-audit"
//	type = "convoy"
//
//	[[legs]]
//	id = "sast"
//	title = "Static Analysis"
//	focus = "Find code vulnerabilities"
//
//	[[legs]]
//	id = "deps"
//	title = "Dependency Audit"
//	focus = "Check for vulnerable dependencies"
//
//	[synthesis]
//	title = "Combine Findings"
//	depends_on = ["sast", "deps"]
//
// Workflow formulas execute steps sequentially with dependencies:
//
//	formula = "release"
//	type = "workflow"
//
//	[[steps]]
//	id = "test"
//	title = "Run Tests"
//
//	[[steps]]
//	id = "build"
//	title = "Build"
//	needs = ["test"]
//
//	[[steps]]
//	id = "publish"
//	title = "Publish"
//	needs = ["build"]
//
// # Validation
//
// The package performs comprehensive validation:
//
//   - Required fields (formula name, valid type)
//   - Unique IDs within steps/legs/templates/aspects
//   - Valid dependency references (needs/depends_on)
//   - Cycle detection in dependency graphs
//
// # Cycle Detection
//
// Workflow and expansion formulas are validated for circular dependencies
// using depth-first search. Cycles are reported with the offending step ID:
//
//	f, err := formula.Parse([]byte(tomlContent))
//	// Returns: "cycle detected involving step: build"
//
// # Topological Sorting
//
// The TopologicalSort method returns steps in dependency order using
// Kahn's algorithm. Dependencies are guaranteed to appear before dependents:
//
//	order, err := f.TopologicalSort()
//	// Returns: ["test", "build", "publish"]
//
// For convoy and aspect formulas (which are parallel), TopologicalSort
// returns all items in their original order.
//
// # Ready Step Computation
//
// The ReadySteps method efficiently computes which steps can execute
// given a set of completed steps:
//
//	completed := map[string]bool{"test": true}
//	ready := f.ReadySteps(completed)
//	// Returns: ["build"] (test is done, build can run)
//
// # Embedded Formulas
//
// The package includes embedded formula files that can be provisioned
// to a beads workspace. Use ProvisionFormulas for initial setup and
// UpdateFormulas for safe updates that preserve user modifications.
//
// # Thread Safety
//
// Formula instances are safe for concurrent read access after parsing.
// The ReadySteps method does not modify state and can be called from
// multiple goroutines with different completed maps.
package formula

package formula

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ParseFile reads and parses a formula.toml file.
func ParseFile(path string) (*Formula, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is from trusted formula directory
	if err != nil {
		return nil, fmt.Errorf("reading formula file: %w", err)
	}
	return Parse(data)
}

// Parse parses formula.toml content from bytes.
func Parse(data []byte) (*Formula, error) {
	var f Formula
	if _, err := toml.Decode(string(data), &f); err != nil {
		return nil, fmt.Errorf("parsing TOML: %w", err)
	}

	// Infer type from content if not explicitly set
	f.inferType()

	if err := f.Validate(); err != nil {
		return nil, err
	}

	return &f, nil
}

// inferType sets the formula type based on content when not explicitly set.
func (f *Formula) inferType() {
	if f.Type != "" {
		return // Type already set
	}

	// Infer from content
	if len(f.Steps) > 0 {
		f.Type = TypeWorkflow
	} else if len(f.Legs) > 0 {
		f.Type = TypeConvoy
	} else if len(f.Template) > 0 {
		f.Type = TypeExpansion
	} else if len(f.Aspects) > 0 {
		f.Type = TypeAspect
	}
}

// Validate checks that the formula has all required fields and valid structure.
func (f *Formula) Validate() error {
	// Check required common fields
	if f.Name == "" {
		return fmt.Errorf("formula field is required")
	}

	if !f.Type.IsValid() {
		return fmt.Errorf("invalid formula type %q (must be convoy, workflow, expansion, or aspect)", f.Type)
	}

	// Type-specific validation
	switch f.Type {
	case TypeConvoy:
		return f.validateConvoy()
	case TypeWorkflow:
		return f.validateWorkflow()
	case TypeExpansion:
		return f.validateExpansion()
	case TypeAspect:
		return f.validateAspect()
	}

	return nil
}

func (f *Formula) validateConvoy() error {
	if len(f.Legs) == 0 {
		return fmt.Errorf("convoy formula requires at least one leg")
	}

	// Check leg IDs are unique
	seen := make(map[string]bool)
	for _, leg := range f.Legs {
		if leg.ID == "" {
			return fmt.Errorf("leg missing required id field")
		}
		if seen[leg.ID] {
			return fmt.Errorf("duplicate leg id: %s", leg.ID)
		}
		seen[leg.ID] = true
	}

	// Validate synthesis depends_on references valid legs
	if f.Synthesis != nil {
		for _, dep := range f.Synthesis.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("synthesis depends_on references unknown leg: %s", dep)
			}
		}
	}

	return nil
}

func (f *Formula) validateWorkflow() error {
	if len(f.Steps) == 0 {
		return fmt.Errorf("workflow formula requires at least one step")
	}

	// Check step IDs are unique
	seen := make(map[string]bool)
	for _, step := range f.Steps {
		if step.ID == "" {
			return fmt.Errorf("step missing required id field")
		}
		if seen[step.ID] {
			return fmt.Errorf("duplicate step id: %s", step.ID)
		}
		seen[step.ID] = true
	}

	// Validate step needs references
	for _, step := range f.Steps {
		for _, need := range step.Needs {
			if !seen[need] {
				return fmt.Errorf("step %q needs unknown step: %s", step.ID, need)
			}
		}
	}

	// Check for cycles
	if err := f.checkCycles(); err != nil {
		return err
	}

	return nil
}

func (f *Formula) validateExpansion() error {
	if len(f.Template) == 0 {
		return fmt.Errorf("expansion formula requires at least one template")
	}

	// Check template IDs are unique
	seen := make(map[string]bool)
	for _, tmpl := range f.Template {
		if tmpl.ID == "" {
			return fmt.Errorf("template missing required id field")
		}
		if seen[tmpl.ID] {
			return fmt.Errorf("duplicate template id: %s", tmpl.ID)
		}
		seen[tmpl.ID] = true
	}

	// Validate template needs references
	for _, tmpl := range f.Template {
		for _, need := range tmpl.Needs {
			if !seen[need] {
				return fmt.Errorf("template %q needs unknown template: %s", tmpl.ID, need)
			}
		}
	}

	return nil
}

func (f *Formula) validateAspect() error {
	if len(f.Aspects) == 0 {
		return fmt.Errorf("aspect formula requires at least one aspect")
	}

	// Check aspect IDs are unique
	seen := make(map[string]bool)
	for _, aspect := range f.Aspects {
		if aspect.ID == "" {
			return fmt.Errorf("aspect missing required id field")
		}
		if seen[aspect.ID] {
			return fmt.Errorf("duplicate aspect id: %s", aspect.ID)
		}
		seen[aspect.ID] = true
	}

	return nil
}

// checkCycles detects circular dependencies in steps.
func (f *Formula) checkCycles() error {
	// Build adjacency list
	deps := make(map[string][]string)
	for _, step := range f.Steps {
		deps[step.ID] = step.Needs
	}

	// DFS for cycle detection
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var visit func(id string) error
	visit = func(id string) error {
		if inStack[id] {
			return fmt.Errorf("cycle detected involving step: %s", id)
		}
		if visited[id] {
			return nil
		}
		visited[id] = true
		inStack[id] = true

		for _, dep := range deps[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}

		inStack[id] = false
		return nil
	}

	for _, step := range f.Steps {
		if err := visit(step.ID); err != nil {
			return err
		}
	}

	return nil
}

// TopologicalSort returns steps in dependency order (dependencies before dependents).
// Only applicable to workflow and expansion formulas.
// Returns an error if there are cycles.
func (f *Formula) TopologicalSort() ([]string, error) {
	var items []string
	var deps map[string][]string

	switch f.Type {
	case TypeWorkflow:
		for _, step := range f.Steps {
			items = append(items, step.ID)
		}
		deps = make(map[string][]string)
		for _, step := range f.Steps {
			deps[step.ID] = step.Needs
		}
	case TypeExpansion:
		for _, tmpl := range f.Template {
			items = append(items, tmpl.ID)
		}
		deps = make(map[string][]string)
		for _, tmpl := range f.Template {
			deps[tmpl.ID] = tmpl.Needs
		}
	case TypeConvoy:
		// Convoy legs are parallel; return all leg IDs
		for _, leg := range f.Legs {
			items = append(items, leg.ID)
		}
		return items, nil
	case TypeAspect:
		// Aspect aspects are parallel; return all aspect IDs
		for _, aspect := range f.Aspects {
			items = append(items, aspect.ID)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported formula type for topological sort")
	}

	// Kahn's algorithm
	inDegree := make(map[string]int)
	for _, id := range items {
		inDegree[id] = 0
	}
	for _, id := range items {
		for _, dep := range deps[id] {
			inDegree[id]++
			_ = dep // dep already exists (validated)
		}
	}

	// Find all nodes with no dependencies
	var queue []string
	for _, id := range items {
		if inDegree[id] == 0 {
			queue = append(queue, id)
		}
	}

	// Build reverse adjacency (who depends on me)
	dependents := make(map[string][]string)
	for _, id := range items {
		for _, dep := range deps[id] {
			dependents[dep] = append(dependents[dep], id)
		}
	}

	var result []string
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]
		result = append(result, id)

		// Reduce in-degree of dependents
		for _, dependent := range dependents[id] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(result) != len(items) {
		return nil, fmt.Errorf("cycle detected in dependencies")
	}

	return result, nil
}

// ReadySteps returns steps that have no unmet dependencies.
// completed is a set of step IDs that have been completed.
func (f *Formula) ReadySteps(completed map[string]bool) []string {
	var ready []string

	switch f.Type {
	case TypeWorkflow:
		for _, step := range f.Steps {
			if completed[step.ID] {
				continue
			}
			allMet := true
			for _, need := range step.Needs {
				if !completed[need] {
					allMet = false
					break
				}
			}
			if allMet {
				ready = append(ready, step.ID)
			}
		}
	case TypeExpansion:
		for _, tmpl := range f.Template {
			if completed[tmpl.ID] {
				continue
			}
			allMet := true
			for _, need := range tmpl.Needs {
				if !completed[need] {
					allMet = false
					break
				}
			}
			if allMet {
				ready = append(ready, tmpl.ID)
			}
		}
	case TypeConvoy:
		// All legs are ready unless already completed
		for _, leg := range f.Legs {
			if !completed[leg.ID] {
				ready = append(ready, leg.ID)
			}
		}
	case TypeAspect:
		// All aspects are ready unless already completed
		for _, aspect := range f.Aspects {
			if !completed[aspect.ID] {
				ready = append(ready, aspect.ID)
			}
		}
	}

	return ready
}

// GetStep returns a step by ID, or nil if not found.
func (f *Formula) GetStep(id string) *Step {
	for i := range f.Steps {
		if f.Steps[i].ID == id {
			return &f.Steps[i]
		}
	}
	return nil
}

// ParallelReadySteps returns ready steps grouped by whether they can run in parallel.
// Returns (parallelSteps, sequentialStep) where:
// - parallelSteps: steps marked with parallel=true that share the same needs
// - sequentialStep: the first non-parallel ready step, or nil if all are parallel
// If multiple parallel steps are ready, they should all be executed concurrently.
func (f *Formula) ParallelReadySteps(completed map[string]bool) (parallel []string, sequential string) {
	ready := f.ReadySteps(completed)
	if len(ready) == 0 {
		return nil, ""
	}

	// For non-workflow formulas, return all as parallel (convoy/aspect are inherently parallel)
	if f.Type != TypeWorkflow {
		return ready, ""
	}

	// Group by parallel flag
	var parallelIDs []string
	var sequentialIDs []string
	for _, id := range ready {
		step := f.GetStep(id)
		if step != nil && step.Parallel {
			parallelIDs = append(parallelIDs, id)
		} else {
			sequentialIDs = append(sequentialIDs, id)
		}
	}

	// If we have parallel steps, return them all for concurrent execution
	if len(parallelIDs) > 0 {
		return parallelIDs, ""
	}

	// Otherwise return the first sequential step
	if len(sequentialIDs) > 0 {
		return nil, sequentialIDs[0]
	}

	return nil, ""
}

// GetLeg returns a leg by ID, or nil if not found.
func (f *Formula) GetLeg(id string) *Leg {
	for i := range f.Legs {
		if f.Legs[i].ID == id {
			return &f.Legs[i]
		}
	}
	return nil
}

// GetTemplate returns a template by ID, or nil if not found.
func (f *Formula) GetTemplate(id string) *Template {
	for i := range f.Template {
		if f.Template[i].ID == id {
			return &f.Template[i]
		}
	}
	return nil
}

// GetAspect returns an aspect by ID, or nil if not found.
func (f *Formula) GetAspect(id string) *Aspect {
	for i := range f.Aspects {
		if f.Aspects[i].ID == id {
			return &f.Aspects[i]
		}
	}
	return nil
}

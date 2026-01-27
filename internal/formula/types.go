// Package formula provides parsing and validation for formula.toml files.
//
// Formulas define structured workflows that can be executed by agents.
// There are four types of formulas:
//   - convoy: Parallel execution of legs with synthesis
//   - workflow: Sequential steps with dependencies
//   - expansion: Template-based step generation
//   - aspect: Multi-aspect parallel analysis (like convoy but for analysis)
package formula

// FormulaType represents the type of formula.
type FormulaType string

const (
	// TypeConvoy is a convoy formula with parallel legs and synthesis.
	TypeConvoy FormulaType = "convoy"
	// TypeWorkflow is a workflow formula with sequential steps.
	TypeWorkflow FormulaType = "workflow"
	// TypeExpansion is an expansion formula with template-based steps.
	TypeExpansion FormulaType = "expansion"
	// TypeAspect is an aspect-based formula for multi-aspect parallel analysis.
	TypeAspect FormulaType = "aspect"
)

// Formula represents a parsed formula.toml file.
type Formula struct {
	// Common fields
	Name        string      `toml:"formula"`
	Description string      `toml:"description"`
	Type        FormulaType `toml:"type"`
	Version     int         `toml:"version"`

	// Convoy-specific
	Inputs    map[string]Input `toml:"inputs"`
	Prompts   map[string]string `toml:"prompts"`
	Output    *Output           `toml:"output"`
	Legs      []Leg             `toml:"legs"`
	Synthesis *Synthesis        `toml:"synthesis"`

	// Workflow-specific
	Steps []Step           `toml:"steps"`
	Vars  map[string]Var   `toml:"vars"`

	// Expansion-specific
	Template []Template `toml:"template"`

	// Aspect-specific (similar to convoy but for analysis)
	Aspects []Aspect `toml:"aspects"`
}

// Aspect represents a parallel analysis aspect in an aspect formula.
type Aspect struct {
	ID          string `toml:"id"`
	Title       string `toml:"title"`
	Focus       string `toml:"focus"`
	Description string `toml:"description"`
}

// Input represents an input parameter for a formula.
type Input struct {
	Description    string   `toml:"description"`
	Type           string   `toml:"type"`
	Required       bool     `toml:"required"`
	RequiredUnless []string `toml:"required_unless"`
	Default        string   `toml:"default"`
}

// Output configures where formula outputs are written.
type Output struct {
	Directory  string `toml:"directory"`
	LegPattern string `toml:"leg_pattern"`
	Synthesis  string `toml:"synthesis"`
}

// Leg represents a parallel execution unit in a convoy formula.
type Leg struct {
	ID          string `toml:"id"`
	Title       string `toml:"title"`
	Focus       string `toml:"focus"`
	Description string `toml:"description"`
}

// Synthesis represents the synthesis step that combines leg outputs.
type Synthesis struct {
	Title       string   `toml:"title"`
	Description string   `toml:"description"`
	DependsOn   []string `toml:"depends_on"`
}

// Step represents a sequential step in a workflow formula.
type Step struct {
	ID          string   `toml:"id"`
	Title       string   `toml:"title"`
	Description string   `toml:"description"`
	Needs       []string `toml:"needs"`
	Parallel    bool     `toml:"parallel"` // If true, this step can run concurrently with other parallel steps that share the same needs
}

// Template represents a template step in an expansion formula.
type Template struct {
	ID          string   `toml:"id"`
	Title       string   `toml:"title"`
	Description string   `toml:"description"`
	Needs       []string `toml:"needs"`
}

// Var represents a variable definition for formulas.
type Var struct {
	Description string `toml:"description"`
	Required    bool   `toml:"required"`
	Default     string `toml:"default"`
}

// IsValid returns true if the formula type is recognized.
func (t FormulaType) IsValid() bool {
	switch t {
	case TypeConvoy, TypeWorkflow, TypeExpansion, TypeAspect:
		return true
	default:
		return false
	}
}

// GetDependencies returns the ordered dependencies for a step/template.
// For convoy formulas, legs are parallel so this returns an empty slice.
// For workflow and expansion formulas, this returns the Needs field.
func (f *Formula) GetDependencies(id string) []string {
	switch f.Type {
	case TypeWorkflow:
		for _, step := range f.Steps {
			if step.ID == id {
				return step.Needs
			}
		}
	case TypeExpansion:
		for _, tmpl := range f.Template {
			if tmpl.ID == id {
				return tmpl.Needs
			}
		}
	case TypeConvoy:
		// Legs are parallel; synthesis depends on all legs
		if f.Synthesis != nil && id == "synthesis" {
			return f.Synthesis.DependsOn
		}
	}
	return nil
}

// GetAllIDs returns all step/leg/template/aspect IDs in the formula.
func (f *Formula) GetAllIDs() []string {
	var ids []string
	switch f.Type {
	case TypeWorkflow:
		for _, step := range f.Steps {
			ids = append(ids, step.ID)
		}
	case TypeExpansion:
		for _, tmpl := range f.Template {
			ids = append(ids, tmpl.ID)
		}
	case TypeConvoy:
		for _, leg := range f.Legs {
			ids = append(ids, leg.ID)
		}
	case TypeAspect:
		for _, aspect := range f.Aspects {
			ids = append(ids, aspect.ID)
		}
	}
	return ids
}

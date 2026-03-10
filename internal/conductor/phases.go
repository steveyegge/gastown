package conductor

// Phase represents a step in the 7-phase development methodology.
type Phase int

const (
	PhaseExamine   Phase = 1 // Architect analyzes codebase
	PhaseHarden    Phase = 2 // Tests artisan adds coverage for target area
	PhaseModernize Phase = 3 // Specialty artisan applies best practices
	PhaseSpecify   Phase = 4 // Tests artisan writes failing specs (user approval gate)
	PhaseImplement Phase = 5 // Specialty artisans write code to pass specs
	PhaseSecure    Phase = 6 // Security artisan reviews
	PhaseDocument  Phase = 7 // Docs artisan writes documentation
)

// PhaseInfo describes a phase of the development methodology.
type PhaseInfo struct {
	Phase       Phase
	Name        string
	Description string
	Owner       string // specialty or role responsible
	Gate        string // what must happen before next phase
	UserGate    bool   // requires explicit user approval
}

// AllPhases returns the ordered list of development phases.
func AllPhases() []PhaseInfo {
	return []PhaseInfo{
		{
			Phase:       PhaseExamine,
			Name:        "examine",
			Description: "Architect analyzes codebase, maps architecture and dependencies",
			Owner:       "architect",
			Gate:        "Architecture report delivered to conductor",
		},
		{
			Phase:       PhaseHarden,
			Name:        "harden",
			Description: "Tests artisan adds coverage for target area (>90% in affected code)",
			Owner:       "tests",
			Gate:        "Hardening PR merged with >90% coverage in target area",
		},
		{
			Phase:       PhaseModernize,
			Name:        "modernize",
			Description: "Specialty artisan applies best practices from ~/.gastown/best-practices/",
			Owner:       "",
			Gate:        "Modernization PR merged",
		},
		{
			Phase:       PhaseSpecify,
			Name:        "specify",
			Description: "Tests artisan writes failing specs based on Architect's API contracts",
			Owner:       "tests",
			Gate:        "Specification PR staged",
			UserGate:    true,
		},
		{
			Phase:       PhaseImplement,
			Name:        "implement",
			Description: "Specialty artisans write code to make failing specs pass",
			Owner:       "",
			Gate:        "All specification tests passing, implementation PR merged",
		},
		{
			Phase:       PhaseSecure,
			Name:        "secure",
			Description: "Security artisan reviews for vulnerabilities and auth issues",
			Owner:       "security",
			Gate:        "Security review PR merged",
		},
		{
			Phase:       PhaseDocument,
			Name:        "document",
			Description: "Docs artisan writes documentation for new/changed functionality",
			Owner:       "docs",
			Gate:        "Documentation PR merged",
		},
	}
}

// PhaseName returns the human-readable name for a phase.
func (p Phase) String() string {
	for _, info := range AllPhases() {
		if info.Phase == p {
			return info.Name
		}
	}
	return "unknown"
}

// GetPhaseInfo returns details about a specific phase.
func GetPhaseInfo(p Phase) *PhaseInfo {
	for _, info := range AllPhases() {
		if info.Phase == p {
			return &info
		}
	}
	return nil
}

// PhaseByName returns the phase constant for a given name.
func PhaseByName(name string) (Phase, bool) {
	for _, info := range AllPhases() {
		if info.Name == name {
			return info.Phase, true
		}
	}
	return 0, false
}

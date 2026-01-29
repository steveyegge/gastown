package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/workspace"
)

// TargetValidationResult contains the result of validating a sling target.
type TargetValidationResult struct {
	Valid       bool     // Whether the target format is valid
	Suggestions []string // Helpful suggestions if invalid
	Reason      string   // Why validation failed
}

// ValidateTarget checks if a sling target has a valid format and provides
// helpful suggestions when the format is ambiguous or incorrect.
//
// Valid target formats:
//   - "." - self (current agent)
//   - "crew" - crew worker in current rig
//   - "<rig>" - rig name (auto-spawn polecat)
//   - "<rig>/polecats/<name>" - specific polecat
//   - "<rig>/crew/<name>" - specific crew worker
//   - "<rig>/witness" - rig witness
//   - "<rig>/refinery" - rig refinery
//   - "mayor" or "may" - mayor
//   - "deacon" or "dea" - deacon
//   - "deacon/dogs" - dog pool
//   - "deacon/dogs/<name>" - specific dog
func ValidateTarget(target string) *TargetValidationResult {
	// Empty target is invalid
	if target == "" {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "target cannot be empty",
			Suggestions: []string{
				"Use '.' to sling to self",
				"Use a rig name like 'gastown' to spawn a new polecat",
				"Use 'mayor' or 'deacon' for town-level agents",
			},
		}
	}

	// "." is always valid (self)
	if target == "." {
		return &TargetValidationResult{Valid: true}
	}

	// Check for known role shortcuts
	lowerTarget := strings.ToLower(target)
	switch lowerTarget {
	case "mayor", "may", "deacon", "dea", "crew", "witness", "wit", "refinery", "ref":
		return &TargetValidationResult{Valid: true}
	}

	// Check for dog targets
	if _, isDog := IsDogTarget(target); isDog {
		return &TargetValidationResult{Valid: true}
	}

	// Check for rig name (no slashes)
	if !strings.Contains(target, "/") {
		// Could be a rig name - let IsRigName validate
		if _, isRig := IsRigName(target); isRig {
			return &TargetValidationResult{Valid: true}
		}
		// Not a valid rig - provide suggestions
		return suggestForUnknownSimpleTarget(target)
	}

	// Path-style target - validate the format
	return validatePathTarget(target)
}

// suggestForUnknownSimpleTarget provides suggestions for a target without slashes
// that isn't a known rig or role.
func suggestForUnknownSimpleTarget(target string) *TargetValidationResult {
	suggestions := []string{}

	// Check if it might be a typo of a known role
	roleHints := map[string]string{
		"mayors":    "mayor",
		"deacons":   "deacon",
		"crews":     "crew",
		"witnesses": "witness",
		"refineries": "refinery",
		"refine":    "refinery",
		"dogs":      "deacon/dogs",
	}
	if hint, ok := roleHints[strings.ToLower(target)]; ok {
		suggestions = append(suggestions, fmt.Sprintf("Did you mean '%s'?", hint))
	}

	// Get available rigs for suggestions
	if rigs := getAvailableRigs(); len(rigs) > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Available rigs: %s", strings.Join(rigs, ", ")))
	}

	suggestions = append(suggestions,
		"Use '<rig>/polecats/<name>' for a specific polecat",
		"Use '<rig>/crew/<name>' for a specific crew worker",
		"Use 'deacon/dogs' to dispatch to the dog pool",
	)

	return &TargetValidationResult{
		Valid:       false,
		Reason:      fmt.Sprintf("'%s' is not a valid rig name or role", target),
		Suggestions: suggestions,
	}
}

// validatePathTarget validates a target with slashes (path-style).
func validatePathTarget(target string) *TargetValidationResult {
	parts := strings.Split(target, "/")

	// Must have at least 2 parts for a valid path
	if len(parts) < 2 {
		return &TargetValidationResult{
			Valid:  false,
			Reason: fmt.Sprintf("invalid target format: '%s'", target),
		}
	}

	// Check for deacon paths
	if strings.ToLower(parts[0]) == "deacon" {
		return validateDeaconPath(parts)
	}

	// Check for mayor paths (mayor doesn't have sub-paths)
	if strings.ToLower(parts[0]) == "mayor" {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "mayor does not have sub-agents",
			Suggestions: []string{
				"Use 'mayor' to target the mayor directly",
			},
		}
	}

	// First part should be a rig name
	rigName := parts[0]

	// Validate the role part
	if len(parts) >= 2 {
		role := strings.ToLower(parts[1])
		switch role {
		case "polecats":
			return validatePolecatPath(parts, rigName)
		case "crew":
			return validateCrewPath(parts, rigName)
		case "witness", "refinery":
			if len(parts) > 2 {
				return &TargetValidationResult{
					Valid:  false,
					Reason: fmt.Sprintf("%s does not have sub-agents", role),
					Suggestions: []string{
						fmt.Sprintf("Use '%s/%s' to target the %s", rigName, role, role),
					},
				}
			}
			return &TargetValidationResult{Valid: true}
		default:
			return suggestForInvalidRole(parts, rigName)
		}
	}

	return &TargetValidationResult{Valid: true}
}

// validateDeaconPath validates paths starting with "deacon".
func validateDeaconPath(parts []string) *TargetValidationResult {
	if len(parts) < 2 {
		return &TargetValidationResult{Valid: true} // Just "deacon" is valid
	}

	subPath := strings.ToLower(parts[1])
	switch subPath {
	case "dogs":
		// "deacon/dogs" or "deacon/dogs/<name>" are valid
		if len(parts) > 3 {
			return &TargetValidationResult{
				Valid:  false,
				Reason: "invalid dog target format",
				Suggestions: []string{
					"Use 'deacon/dogs' for any idle dog",
					"Use 'deacon/dogs/<name>' for a specific dog",
				},
			}
		}
		return &TargetValidationResult{Valid: true}
	default:
		return &TargetValidationResult{
			Valid:  false,
			Reason: fmt.Sprintf("deacon has no '%s' sub-path", subPath),
			Suggestions: []string{
				"Use 'deacon/dogs' to dispatch to dogs",
				"Use 'deacon' to target the deacon directly",
			},
		}
	}
}

// validatePolecatPath validates <rig>/polecats/<name> paths.
func validatePolecatPath(parts []string, rigName string) *TargetValidationResult {
	if len(parts) < 3 {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "polecat name is required",
			Suggestions: []string{
				fmt.Sprintf("Use '%s/polecats/<name>' for a specific polecat", rigName),
				fmt.Sprintf("Use '%s' to auto-spawn a new polecat", rigName),
			},
		}
	}
	if len(parts) > 3 {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "too many path segments for polecat target",
			Suggestions: []string{
				fmt.Sprintf("Use '%s/polecats/%s'", rigName, parts[2]),
			},
		}
	}
	if parts[2] == "" {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "polecat name cannot be empty",
			Suggestions: []string{
				fmt.Sprintf("Use '%s/polecats/<name>' for a specific polecat", rigName),
			},
		}
	}
	return &TargetValidationResult{Valid: true}
}

// validateCrewPath validates <rig>/crew/<name> paths.
func validateCrewPath(parts []string, rigName string) *TargetValidationResult {
	if len(parts) < 3 {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "crew member name is required",
			Suggestions: []string{
				fmt.Sprintf("Use '%s/crew/<name>' for a specific crew member", rigName),
				"Use 'crew' to target a crew worker in your current rig",
			},
		}
	}
	if len(parts) > 3 {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "too many path segments for crew target",
			Suggestions: []string{
				fmt.Sprintf("Use '%s/crew/%s'", rigName, parts[2]),
			},
		}
	}
	if parts[2] == "" {
		return &TargetValidationResult{
			Valid:  false,
			Reason: "crew member name cannot be empty",
			Suggestions: []string{
				fmt.Sprintf("Use '%s/crew/<name>' for a specific crew member", rigName),
			},
		}
	}
	return &TargetValidationResult{Valid: true}
}

// suggestForInvalidRole provides suggestions when the role part of a path is invalid.
func suggestForInvalidRole(parts []string, rigName string) *TargetValidationResult {
	invalidRole := parts[1]
	suggestions := []string{}

	// Check for common typos
	typoHints := map[string]string{
		"polecat":   "polecats",
		"workers":   "polecats",
		"worker":    "polecats",
		"agents":    "polecats",
		"agent":     "polecats",
		"crews":     "crew",
		"witnesses": "witness",
		"wit":       "witness",
		"refineries": "refinery",
		"ref":       "refinery",
	}

	if hint, ok := typoHints[strings.ToLower(invalidRole)]; ok {
		suggestions = append(suggestions, fmt.Sprintf("Did you mean '%s/%s'?", rigName, hint))
	}

	suggestions = append(suggestions,
		fmt.Sprintf("Valid roles for rig '%s': polecats, crew, witness, refinery", rigName),
		fmt.Sprintf("Example: '%s/polecats/<name>' or '%s/crew/<name>'", rigName, rigName),
	)

	return &TargetValidationResult{
		Valid:       false,
		Reason:      fmt.Sprintf("'%s' is not a valid role (in '%s')", invalidRole, strings.Join(parts, "/")),
		Suggestions: suggestions,
	}
}

// getAvailableRigs returns a list of available rig names for suggestions.
func getAvailableRigs() []string {
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return nil
	}

	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		return nil
	}

	var rigs []string
	for name := range rigsConfig.Rigs {
		rigs = append(rigs, name)
	}
	return rigs
}

// FormatValidationError formats a validation result as a user-friendly error.
func FormatValidationError(result *TargetValidationResult) string {
	if result.Valid {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(result.Reason)

	if len(result.Suggestions) > 0 {
		sb.WriteString("\n\nSuggestions:\n")
		for _, s := range result.Suggestions {
			sb.WriteString("  - ")
			sb.WriteString(s)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

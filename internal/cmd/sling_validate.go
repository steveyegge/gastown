package cmd

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/witness"
)

// knownRoles lists valid second-segment roles in path-style sling targets.
var knownRoles = map[string]bool{
	"polecats": true,
	"crew":     true,
	"witness":  true,
	"refinery": true,
}

// ValidateTarget performs lightweight pre-checks on a sling target string,
// catching common mistakes before resolveTarget can trigger side-effects
// like polecat spawning. It returns a non-nil error with a helpful message
// when the target is clearly malformed.
//
// It intentionally does NOT duplicate the full resolution logic — valid
// targets that pass this check are still resolved by resolveTarget.
func ValidateTarget(target string) error {
	// Self, empty, and role shortcuts are always fine.
	if target == "" || target == "." {
		return nil
	}

	// No slashes → could be rig name or role shortcut; let resolveTarget decide.
	if !strings.Contains(target, "/") {
		return nil
	}

	parts := strings.Split(target, "/")

	// Reject empty segments: "rig//polecats", "/polecats", "rig/polecats/"
	for i, p := range parts {
		if p == "" {
			return fmt.Errorf("invalid target %q: empty path segment at position %d\n"+
				"Valid formats:\n"+
				"  <rig>                  auto-spawn polecat\n"+
				"  <rig>/polecats/<name>  specific polecat\n"+
				"  <rig>/crew/<name>      crew worker\n"+
				"  <rig>/witness          rig witness\n"+
				"  <rig>/refinery         rig refinery\n"+
				"  deacon/dogs            dog pool\n"+
				"  mayor                  town mayor",
				target, i)
		}
	}

	// Dog targets are valid at any depth (deacon/dogs, deacon/dogs/<name>).
	// Deacon sub-path validation is handled downstream by IsDogTarget/resolveTarget.
	if strings.ToLower(parts[0]) == "deacon" {
		return nil
	}

	// Mayor has no sub-agents.
	if strings.ToLower(parts[0]) == "mayor" {
		return fmt.Errorf("invalid target %q: mayor does not have sub-agents\n"+
			"Use 'mayor' to target the mayor directly", target)
	}

	// Path targets: parts[0] = rig, parts[1] = role or shorthand name.
	// Two-segment paths like "gastown/nux" are polecat/crew shorthand —
	// resolvePathToSession handles these by trying polecat then crew lookup.
	// We only validate when the second segment IS a known role.
	if len(parts) >= 2 {
		role := strings.ToLower(parts[1])
		if knownRoles[role] {
			// Known role: apply role-specific constraints.
			if role == "witness" || role == "refinery" {
				// Witness and refinery are singleton roles — no sub-agents.
				if len(parts) > 2 {
					return fmt.Errorf("invalid target %q: %s does not have named sub-agents\n"+
						"Usage: %s/%s", target, role, parts[0], role)
				}
			} else if len(parts) == 2 {
				// Crew and polecats require a name segment.
				if role == "crew" {
					return fmt.Errorf("invalid target %q: crew requires a worker name\n"+
						"Usage: %s/crew/<name>", target, parts[0])
				}
				return fmt.Errorf("invalid target %q: polecats requires a polecat name\n"+
					"Usage: %s/polecats/<name>\n"+
					"Or use just %q to auto-spawn a polecat", target, parts[0], parts[0])
			}
			// Too many segments for role paths: rig/role/name/extra
			if len(parts) > 3 {
				return fmt.Errorf("invalid target %q: too many path segments (max 3: rig/role/name)", target)
			}
		} else if len(parts) > 2 {
			// Not a known role but has 3+ segments — not a valid shorthand.
			return fmt.Errorf("invalid target %q: unknown role %q\n"+
				"Valid roles after a rig name:\n"+
				"  %s/polecats/<name>  specific polecat\n"+
				"  %s/crew/<name>      crew worker\n"+
				"  %s/witness          rig witness\n"+
				"  %s/refinery         rig refinery\n"+
				"Or use just %q to target by name shorthand",
				target, parts[1], parts[0], parts[0], parts[0], parts[0], parts[0])
		}
		// else: 2-segment with unknown role → polecat/crew shorthand, let resolveTarget handle.
	}

	return nil
}

// slingCheck records a single validation check result for dry-run reporting.
type slingCheck struct {
	Name   string // Short label (e.g., "bead exists")
	Status string // "pass", "info", "warn"
	Detail string // Optional detail
}

// slingChecklist accumulates validation checks during a dry-run sling.
type slingChecklist struct {
	checks []slingCheck
}

func (c *slingChecklist) pass(name, detail string) {
	c.checks = append(c.checks, slingCheck{Name: name, Status: "pass", Detail: detail})
}

func (c *slingChecklist) info(name, detail string) {
	c.checks = append(c.checks, slingCheck{Name: name, Status: "info", Detail: detail})
}

func (c *slingChecklist) warn(name, detail string) {
	c.checks = append(c.checks, slingCheck{Name: name, Status: "warn", Detail: detail})
}

// render prints the validation checklist and a summary line.
func (c *slingChecklist) render() {
	if len(c.checks) == 0 {
		return
	}

	fmt.Printf("\n%s\n", style.Bold.Render("Validation"))
	passCount := 0
	for _, chk := range c.checks {
		icon := style.Success.Render("✓")
		switch chk.Status {
		case "info":
			icon = style.Info.Render("·")
		case "warn":
			icon = style.Warning.Render("!")
		default:
			passCount++
		}
		if chk.Detail != "" {
			fmt.Printf("  %s %-24s %s\n", icon, chk.Name, style.Dim.Render(chk.Detail))
		} else {
			fmt.Printf("  %s %s\n", icon, chk.Name)
		}
	}

	fmt.Printf("\n%s %d/%d checks passed\n", style.Bold.Render("Result:"), passCount, len(c.checks))
}

// renderDryRunPlan prints the planned operations.
func renderDryRunPlan(lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Printf("\n%s\n", style.Bold.Render("Plan"))
	for _, line := range lines {
		fmt.Printf("  %s %s\n", style.Dim.Render("→"), line)
	}
}

// buildSlingValidation builds a validation checklist for a single-bead sling.
// Called from the dry-run output block after all validation has already passed.
func buildSlingValidation(beadID string, info *beadInfo, targetAgent, formulaName, townRoot string, force bool) *slingChecklist {
	cl := &slingChecklist{}

	cl.pass("bead exists", beadID)

	statusDetail := info.Status
	if info.Assignee != "" && (info.Status == "hooked" || info.Status == "in_progress") {
		statusDetail = fmt.Sprintf("%s → %s (force=%v)", info.Status, info.Assignee, force)
	}
	cl.pass("bead status", statusDetail)
	cl.pass("title valid", truncate(info.Title, 50))
	cl.pass("target resolved", targetAgent)

	if formulaName != "" {
		cl.pass("formula exists", formulaName)
	}
	if strings.Contains(targetAgent, "/polecats/") {
		cl.pass("cross-rig guard", "prefix matches target rig")
	}

	// Extra dry-run-only check: respawn count.
	if witness.ShouldBlockRespawn(townRoot, beadID) {
		cl.warn("respawn count", fmt.Sprintf("at limit — run: gt sling respawn-reset %s", beadID))
	} else {
		cl.pass("respawn count", "within limit")
	}

	return cl
}

// buildScheduleValidation builds a validation checklist for a scheduled sling.
func buildScheduleValidation(beadID string, info *beadInfo, rigName, formulaName string) *slingChecklist {
	cl := &slingChecklist{}

	cl.pass("bead exists", beadID)
	cl.pass("bead status", info.Status)
	cl.pass("target rig", rigName)

	if formulaName != "" {
		cl.pass("formula exists", formulaName)
	}

	cl.pass("cross-rig guard", "prefix matches target rig")
	cl.pass("no duplicate schedule", "no open sling context")

	return cl
}

// buildDryRunPlan builds the list of planned operations for dry-run output.
func buildDryRunPlan(beadID string, info *beadInfo, targetAgent, targetPane, formulaName string, opts dryRunPlanOpts) []string {
	var plan []string

	if formulaName != "" {
		plan = append(plan, fmt.Sprintf("Cook formula: %s", formulaName))
		plan = append(plan,
			fmt.Sprintf("Create wisp: bd mol wisp %s --var feature=%q --var issue=%q",
				formulaName, truncate(info.Title, 30), beadID))
		plan = append(plan, fmt.Sprintf("Bond wisp to %s", beadID))
	}

	if !opts.noConvoy {
		if tracked := isTrackedByConvoy(beadID); tracked != "" {
			plan = append(plan, fmt.Sprintf("Already tracked by convoy %s", tracked))
		} else {
			plan = append(plan, fmt.Sprintf("Create auto-convoy: \"Work: %s\"", truncate(info.Title, 40)))
		}
	}

	plan = append(plan, fmt.Sprintf("Hook bead: bd update %s --status=hooked --assignee=%s", beadID, targetAgent))

	if opts.args != "" {
		plan = append(plan, fmt.Sprintf("Store args: %s", truncate(opts.args, 60)))
	}
	if opts.merge != "" {
		plan = append(plan, fmt.Sprintf("Merge strategy: %s", opts.merge))
	}
	if opts.noMerge {
		plan = append(plan, "No-merge mode (work stays on feature branch)")
	}

	if targetPane != "" {
		plan = append(plan, fmt.Sprintf("Nudge pane: %s", targetPane))
	} else {
		plan = append(plan, "Agent discovers work via gt prime")
	}

	return plan
}

type dryRunPlanOpts struct {
	args     string
	merge    string
	noConvoy bool
	noMerge  bool
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

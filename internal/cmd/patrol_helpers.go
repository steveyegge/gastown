package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/style"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// PatrolConfig holds role-specific patrol configuration.
type PatrolConfig struct {
	RoleName      string   // "deacon", "witness", "refinery"
	PatrolMolName string   // "mol-deacon-patrol", etc.
	BeadsDir      string   // where to look for beads
	Assignee      string   // agent identity for pinning
	HeaderEmoji   string   // display emoji
	HeaderTitle   string   // "Patrol Status", etc.
	WorkLoopSteps []string // role-specific instructions
	CheckInProgress bool   // whether to check in_progress status first (witness/refinery do, deacon doesn't)
}

// findActivePatrol finds an active patrol molecule for the role.
// Returns the patrol ID, display line, and whether one was found.
func findActivePatrol(cfg PatrolConfig) (patrolID, patrolLine string, found bool) {
	// Check for in-progress patrol first (if configured)
	if cfg.CheckInProgress {
		cmdList := exec.Command("bd", "--no-daemon", "list", "--status=in_progress", "--type=epic")
		cmdList.Dir = cfg.BeadsDir
		var stdoutList, stderrList bytes.Buffer
		cmdList.Stdout = &stdoutList
		cmdList.Stderr = &stderrList

		if err := cmdList.Run(); err != nil {
			if errMsg := strings.TrimSpace(stderrList.String()); errMsg != "" {
				fmt.Fprintf(os.Stderr, "bd list: %s\n", errMsg)
			}
		} else {
			lines := strings.Split(stdoutList.String(), "\n")
			for _, line := range lines {
				if strings.Contains(line, cfg.PatrolMolName) && !strings.Contains(line, "[template]") {
					parts := strings.Fields(line)
					if len(parts) > 0 {
						return parts[0], line, true
					}
				}
			}
		}
	}

	// Check for open patrols with open children (active wisp)
	cmdOpen := exec.Command("bd", "--no-daemon", "list", "--status=open", "--type=epic")
	cmdOpen.Dir = cfg.BeadsDir
	var stdoutOpen, stderrOpen bytes.Buffer
	cmdOpen.Stdout = &stdoutOpen
	cmdOpen.Stderr = &stderrOpen

	if err := cmdOpen.Run(); err != nil {
		if errMsg := strings.TrimSpace(stderrOpen.String()); errMsg != "" {
			fmt.Fprintf(os.Stderr, "bd list: %s\n", errMsg)
		}
	} else {
		lines := strings.Split(stdoutOpen.String(), "\n")
		for _, line := range lines {
			if strings.Contains(line, cfg.PatrolMolName) && !strings.Contains(line, "[template]") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					molID := parts[0]
					// Check if this molecule has open children
					cmdShow := exec.Command("bd", "--no-daemon", "show", molID)
					cmdShow.Dir = cfg.BeadsDir
					var stdoutShow, stderrShow bytes.Buffer
					cmdShow.Stdout = &stdoutShow
					cmdShow.Stderr = &stderrShow
					if err := cmdShow.Run(); err != nil {
						if errMsg := strings.TrimSpace(stderrShow.String()); errMsg != "" {
							fmt.Fprintf(os.Stderr, "bd show: %s\n", errMsg)
						}
					} else {
						showOutput := stdoutShow.String()
						// Deacon only checks "- open]", witness/refinery also check "- in_progress]"
						hasOpenChildren := strings.Contains(showOutput, "- open]")
						if cfg.CheckInProgress {
							hasOpenChildren = hasOpenChildren || strings.Contains(showOutput, "- in_progress]")
						}
						if hasOpenChildren {
							return molID, line, true
						}
					}
				}
			}
		}
	}

	return "", "", false
}

// autoSpawnPatrol creates and pins a new patrol wisp.
// Returns the patrol ID or an error.
func autoSpawnPatrol(cfg PatrolConfig) (string, error) {
	// Find the proto ID for the patrol molecule
	cmdCatalog := exec.Command("bd", "--no-daemon", "mol", "catalog")
	cmdCatalog.Dir = cfg.BeadsDir
	var stdoutCatalog, stderrCatalog bytes.Buffer
	cmdCatalog.Stdout = &stdoutCatalog
	cmdCatalog.Stderr = &stderrCatalog

	if err := cmdCatalog.Run(); err != nil {
		errMsg := strings.TrimSpace(stderrCatalog.String())
		if errMsg != "" {
			return "", fmt.Errorf("failed to list molecule catalog: %s", errMsg)
		}
		return "", fmt.Errorf("failed to list molecule catalog: %w", err)
	}

	// Find patrol molecule in catalog
	var protoID string
	catalogLines := strings.Split(stdoutCatalog.String(), "\n")
	for _, line := range catalogLines {
		if strings.Contains(line, cfg.PatrolMolName) {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				// Strip trailing colon from ID (catalog format: "gt-xxx: title")
				protoID = strings.TrimSuffix(parts[0], ":")
				break
			}
		}
	}

	if protoID == "" {
		return "", fmt.Errorf("proto %s not found in catalog", cfg.PatrolMolName)
	}

	// Create the patrol wisp
	cmdSpawn := exec.Command("bd", "--no-daemon", "mol", "wisp", "create", protoID, "--actor", cfg.RoleName)
	cmdSpawn.Dir = cfg.BeadsDir
	var stdoutSpawn, stderrSpawn bytes.Buffer
	cmdSpawn.Stdout = &stdoutSpawn
	cmdSpawn.Stderr = &stderrSpawn

	if err := cmdSpawn.Run(); err != nil {
		return "", fmt.Errorf("failed to create patrol wisp: %s", stderrSpawn.String())
	}

	// Parse the created molecule ID from output
	var patrolID string
	spawnOutput := stdoutSpawn.String()
	for _, line := range strings.Split(spawnOutput, "\n") {
		if strings.Contains(line, "Root issue:") || strings.Contains(line, "Created") {
			parts := strings.Fields(line)
			for _, p := range parts {
				if strings.HasPrefix(p, "wisp-") || strings.HasPrefix(p, "gt-") {
					patrolID = p
					break
				}
			}
		}
	}

	if patrolID == "" {
		return "", fmt.Errorf("created wisp but could not parse ID from output")
	}

	// Hook the wisp to the agent so gt mol status sees it
	cmdPin := exec.Command("bd", "--no-daemon", "update", patrolID, "--status=hooked", "--assignee="+cfg.Assignee)
	cmdPin.Dir = cfg.BeadsDir
	if err := cmdPin.Run(); err != nil {
		return patrolID, fmt.Errorf("created wisp %s but failed to hook", patrolID)
	}

	// Also set the hook slot on the agent bead (fixes mi-619: hook slot consistency bug)
	// This is critical - the agent bead's hook slot is what gt hook queries.
	// Without this, the patrol wisp status is "hooked" but the agent's hook slot is empty,
	// making the work invisible to gt hook and breaking autonomous execution.
	setPatrolAgentHookSlot(patrolID, cfg)

	return patrolID, nil
}

// outputPatrolContext is the main function that handles patrol display logic.
// It finds or creates a patrol and outputs the status and work loop.
func outputPatrolContext(cfg PatrolConfig) {
	fmt.Println()
	fmt.Printf("%s\n\n", style.Bold.Render(fmt.Sprintf("## %s %s", cfg.HeaderEmoji, cfg.HeaderTitle)))

	// Try to find an active patrol
	patrolID, patrolLine, hasPatrol := findActivePatrol(cfg)

	if !hasPatrol {
		// No active patrol - auto-spawn one
		fmt.Printf("Status: **No active patrol** - creating %s...\n", cfg.PatrolMolName)
		fmt.Println()

		var err error
		patrolID, err = autoSpawnPatrol(cfg)
		if err != nil {
			if patrolID != "" {
				fmt.Printf("⚠ %s\n", err.Error())
			} else {
				fmt.Println(style.Dim.Render(err.Error()))
				fmt.Println(style.Dim.Render(fmt.Sprintf("Run `bd mol catalog` to troubleshoot.")))
				return
			}
		} else {
			fmt.Printf("✓ Created and hooked patrol wisp: %s\n", patrolID)
		}
	} else {
		// Has active patrol - show status
		fmt.Println("Status: **Patrol Active**")
		fmt.Printf("Patrol: %s\n\n", strings.TrimSpace(patrolLine))
	}

	// Show patrol work loop instructions
	fmt.Printf("**%s Patrol Work Loop:**\n", cases.Title(language.English).String(cfg.RoleName))
	for i, step := range cfg.WorkLoopSteps {
		fmt.Printf("%d. %s\n", i+1, step)
	}

	if patrolID != "" {
		fmt.Println()
		fmt.Printf("Current patrol ID: %s\n", patrolID)
	}
}

// setPatrolAgentHookSlot sets the hook slot on the agent bead for a patrol.
// This fixes the mi-619 bug where the patrol wisp is marked hooked but the agent
// bead's hook slot is empty, making work invisible to gt hook.
func setPatrolAgentHookSlot(patrolID string, cfg PatrolConfig) {
	var agentBeadID string

	switch cfg.RoleName {
	case "deacon":
		agentBeadID = beads.DeaconBeadIDTown()
	case "witness":
		// Extract rig from assignee (format: "rig/witness")
		parts := strings.Split(cfg.Assignee, "/")
		if len(parts) >= 2 && parts[1] == "witness" {
			rig := parts[0]
			prefix := beads.GetPrefixForRig("", rig) // "" since we can infer from beads dir
			agentBeadID = beads.WitnessBeadIDWithPrefix(prefix, rig)
		}
	case "refinery":
		// Extract rig from assignee (format: "rig/refinery")
		parts := strings.Split(cfg.Assignee, "/")
		if len(parts) >= 2 && parts[1] == "refinery" {
			rig := parts[0]
			prefix := beads.GetPrefixForRig("", rig)
			agentBeadID = beads.RefineryBeadIDWithPrefix(prefix, rig)
		}
	}

	if agentBeadID == "" {
		// Silent skip - some configurations may not support hook slot setting
		return
	}

	cmdHookSlot := exec.Command("bd", "--no-daemon", "slot", "set", agentBeadID, "hook", patrolID)
	cmdHookSlot.Dir = cfg.BeadsDir
	if err := cmdHookSlot.Run(); err != nil {
		// Non-fatal: the work is still hooked (status=hooked), but the agent bead's hook slot
		// might not be updated. Log but continue - bd slot set can fail for cross-database scenarios.
		fmt.Fprintf(os.Stderr, "Warning: could not set agent hook slot for %s: %v\n", cfg.Assignee, err)
	}
}

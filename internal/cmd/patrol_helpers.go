package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

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
	// Check for hooked patrol first (highest priority - resume it)
	cmdHooked := exec.Command("bd", "--no-daemon", "list", "--status=hooked", "--type=epic")
	cmdHooked.Dir = cfg.BeadsDir
	var stdoutHooked, stderrHooked bytes.Buffer
	cmdHooked.Stdout = &stdoutHooked
	cmdHooked.Stderr = &stderrHooked

	if err := cmdHooked.Run(); err != nil {
		if errMsg := strings.TrimSpace(stderrHooked.String()); errMsg != "" {
			fmt.Fprintf(os.Stderr, "bd list: %s\n", errMsg)
		}
	} else {
		lines := strings.Split(stdoutHooked.String(), "\n")
		for _, line := range lines {
			if strings.Contains(line, cfg.PatrolMolName) && !strings.Contains(line, "[template]") {
				parts := strings.Fields(line)
				if len(parts) > 0 {
					return parts[0], line, true
				}
			}
		}
	}

	// Check for in-progress patrol (if configured)
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

// findStalePatrols finds patrol molecules that should be closed before creating a new one.
// Returns a list of patrol IDs that are stale (open but with no active children).
func findStalePatrols(cfg PatrolConfig) []string {
	var stalePatrols []string

	// Find open patrols without active children
	cmdOpen := exec.Command("bd", "--no-daemon", "list", "--status=open", "--type=epic")
	cmdOpen.Dir = cfg.BeadsDir
	var stdoutOpen, stderrOpen bytes.Buffer
	cmdOpen.Stdout = &stdoutOpen
	cmdOpen.Stderr = &stderrOpen

	if err := cmdOpen.Run(); err != nil {
		return stalePatrols
	}

	lines := strings.Split(stdoutOpen.String(), "\n")
	for _, line := range lines {
		if strings.Contains(line, cfg.PatrolMolName) && !strings.Contains(line, "[template]") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				molID := parts[0]
				// Check if this molecule has active children
				cmdShow := exec.Command("bd", "--no-daemon", "show", molID)
				cmdShow.Dir = cfg.BeadsDir
				var stdoutShow, stderrShow bytes.Buffer
				cmdShow.Stdout = &stdoutShow
				cmdShow.Stderr = &stderrShow
				if err := cmdShow.Run(); err == nil {
					showOutput := stdoutShow.String()
					hasOpenChildren := strings.Contains(showOutput, "- open]")
					hasHookedChildren := strings.Contains(showOutput, "- hooked]")
					hasInProgressChildren := strings.Contains(showOutput, "- in_progress]")
					if !hasOpenChildren && !hasHookedChildren && !hasInProgressChildren {
						stalePatrols = append(stalePatrols, molID)
					}
				}
			}
		}
	}

	return stalePatrols
}

// closeStalePatrols closes stale patrol molecules and their children.
func closeStalePatrols(cfg PatrolConfig) {
	stalePatrols := findStalePatrols(cfg)
	for _, molID := range stalePatrols {
		// Close all children first
		cmdChildren := exec.Command("bd", "--no-daemon", "list", "--parent", molID)
		cmdChildren.Dir = cfg.BeadsDir
		var stdoutChildren bytes.Buffer
		cmdChildren.Stdout = &stdoutChildren
		if err := cmdChildren.Run(); err == nil {
			childLines := strings.Split(stdoutChildren.String(), "\n")
			for _, childLine := range childLines {
				childParts := strings.Fields(childLine)
				if len(childParts) > 0 {
					childID := childParts[0]
					cmdClose := exec.Command("bd", "--no-daemon", "close", childID)
					cmdClose.Dir = cfg.BeadsDir
					cmdClose.Run() // Ignore errors - child may already be closed
				}
			}
		}
		// Close the patrol molecule itself
		cmdClose := exec.Command("bd", "--no-daemon", "close", molID)
		cmdClose.Dir = cfg.BeadsDir
		cmdClose.Run() // Ignore errors
	}
}

// autoSpawnPatrol creates and pins a new patrol wisp.
// Returns the patrol ID or an error.
func autoSpawnPatrol(cfg PatrolConfig) (string, error) {
	// Find the proto ID for the patrol molecule
	cmdCatalog := exec.Command("gt", "formula", "list")
	cmdCatalog.Dir = cfg.BeadsDir
	var stdoutCatalog, stderrCatalog bytes.Buffer
	cmdCatalog.Stdout = &stdoutCatalog
	cmdCatalog.Stderr = &stderrCatalog

	if err := cmdCatalog.Run(); err != nil {
		errMsg := strings.TrimSpace(stderrCatalog.String())
		if errMsg != "" {
			return "", fmt.Errorf("failed to list formulas: %s", errMsg)
		}
		return "", fmt.Errorf("failed to list formulas: %w", err)
	}

	// Find patrol molecule in formula list
	// Format: "formula-name         description"
	var protoID string
	catalogLines := strings.Split(stdoutCatalog.String(), "\n")
	for _, line := range catalogLines {
		if strings.Contains(line, cfg.PatrolMolName) {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				protoID = parts[0]
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
		// No active patrol - close any stale patrols before creating new one
		closeStalePatrols(cfg)

		// Auto-spawn a new patrol
		fmt.Printf("Status: **No active patrol** - creating %s...\n", cfg.PatrolMolName)
		fmt.Println()

		var err error
		patrolID, err = autoSpawnPatrol(cfg)
		if err != nil {
			if patrolID != "" {
				fmt.Printf("⚠ %s\n", err.Error())
			} else {
				fmt.Println(style.Dim.Render(err.Error()))
				fmt.Println(style.Dim.Render(fmt.Sprintf("Run `gt formula list` to troubleshoot.")))
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

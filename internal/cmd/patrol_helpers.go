package cmd

import (
	"github.com/steveyegge/gastown/internal/cli"
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

	// Check for hooked patrols first — autoSpawnPatrol sets status to hooked,
	// so this is the most common state for an active patrol molecule.
	if id, line, ok := findPatrolByStatus(cfg, "hooked"); ok {
		return id, line, true
	}

	// Check for open patrols with open children (active wisp)
	if id, line, ok := findPatrolByStatus(cfg, "open"); ok {
		return id, line, true
	}

	return "", "", false
}

// findPatrolByStatus searches for a patrol molecule with the given status.
func findPatrolByStatus(cfg PatrolConfig, status string) (patrolID, patrolLine string, found bool) {
	cmdList := exec.Command("bd", "--no-daemon", "list", "--status="+status, "--type=epic")
	cmdList.Dir = cfg.BeadsDir
	var stdoutList, stderrList bytes.Buffer
	cmdList.Stdout = &stdoutList
	cmdList.Stderr = &stderrList

	if err := cmdList.Run(); err != nil {
		if errMsg := strings.TrimSpace(stderrList.String()); errMsg != "" {
			fmt.Fprintf(os.Stderr, "bd list: %s\n", errMsg)
		}
		return "", "", false
	}

	lines := strings.Split(stdoutList.String(), "\n")
	for _, line := range lines {
		if strings.Contains(line, cfg.PatrolMolName) && !strings.Contains(line, "[template]") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				return parts[0], line, true
			}
		}
	}
	return "", "", false
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
				fmt.Println(style.Dim.Render("Run `" + cli.Name() + " formula list` to troubleshoot."))
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

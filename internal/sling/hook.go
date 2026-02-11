package sling

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/bdcmd"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/workspace"
)

// HookBead hooks a bead to a target agent with retry logic.
// Returns nil on success or error after max retries.
func HookBead(beadID, targetAgent, townRoot, hookWorkDir string, out io.Writer) error {
	hookDir := beads.ResolveHookDir(townRoot, beadID, hookWorkDir)
	const maxRetries = 3
	skipVerify := os.Getenv("GT_TEST_SKIP_HOOK_VERIFY") != ""

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		hookCmd := bdcmd.Command("update", beadID, "--status=hooked", "--assignee="+targetAgent)
		hookCmd.Dir = hookDir
		hookCmd.Stderr = os.Stderr
		if err := hookCmd.Run(); err != nil {
			lastErr = err
			if attempt < maxRetries {
				backoff := time.Duration(attempt*500) * time.Millisecond
				fmt.Fprintf(out, "Hook attempt %d failed, retrying in %v...\n", attempt, backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("hooking bead after %d attempts: %w", maxRetries, err)
		}

		if skipVerify {
			break
		}

		// Verify the hook actually stuck
		verifyInfo, verifyErr := GetBeadInfo(beadID, townRoot)
		if verifyErr != nil {
			lastErr = fmt.Errorf("verifying hook: %w", verifyErr)
			if attempt < maxRetries {
				backoff := time.Duration(attempt*500) * time.Millisecond
				fmt.Fprintf(out, "Hook verification failed, retrying in %v...\n", backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("verifying hook after %d attempts: %w", maxRetries, lastErr)
		}

		if verifyInfo.Status != "hooked" || verifyInfo.Assignee != targetAgent {
			lastErr = fmt.Errorf("hook did not stick: status=%s, assignee=%s (expected hooked, %s)",
				verifyInfo.Status, verifyInfo.Assignee, targetAgent)
			if attempt < maxRetries {
				backoff := time.Duration(attempt*500) * time.Millisecond
				fmt.Fprintf(out, "%v, retrying in %v...\n", lastErr, backoff)
				time.Sleep(backoff)
				continue
			}
			return fmt.Errorf("hook failed after %d attempts: %w", maxRetries, lastErr)
		}

		break
	}

	return nil
}

// UpdateAgentHookBead updates the agent bead's state and hook when work is slung.
func UpdateAgentHookBead(agentID, beadID, workDir, townBeadsDir string) {
	_ = townBeadsDir

	bdWorkDir := workDir
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't find town root to update agent hook: %v\n", err)
		return
	}
	if bdWorkDir == "" {
		bdWorkDir = townRoot
	}

	agentBeadID := AgentIDToBeadID(agentID, townRoot)
	if agentBeadID == "" {
		return
	}

	agentWorkDir := beads.ResolveHookDir(townRoot, agentBeadID, bdWorkDir)

	bd := beads.New(agentWorkDir)
	if err := bd.SetHookBead(agentBeadID, beadID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: couldn't set agent %s hook: %v\n", agentBeadID, err)
		if strings.Contains(agentBeadID, "-dog-") {
			fmt.Fprintf(os.Stderr, "  (Old dog? Recreate with: gt dog rm <name> && gt dog add <name>)\n")
		}
		return
	}
}

// AgentIDToBeadID converts an agent ID to its corresponding agent bead ID.
func AgentIDToBeadID(agentID, townRoot string) string {
	agentID = strings.TrimSuffix(agentID, "/")

	if agentID == "mayor" {
		return beads.MayorBeadIDTown()
	}
	if agentID == "deacon" {
		return beads.DeaconBeadIDTown()
	}

	parts := strings.Split(agentID, "/")
	if len(parts) < 2 {
		return ""
	}

	rigName := parts[0]

	townName, err := workspace.GetTownName(townRoot)
	if err != nil {
		townName = ""
	}

	switch {
	case len(parts) == 2 && parts[1] == "witness":
		return beads.WitnessBeadIDTown(townName, rigName)
	case len(parts) == 2 && parts[1] == "refinery":
		return beads.RefineryBeadIDTown(townName, rigName)
	case len(parts) == 3 && parts[1] == "crew":
		return beads.CrewBeadIDTown(townName, rigName, parts[2])
	case len(parts) == 3 && parts[1] == "polecats":
		prefix := beads.GetPrefixForRig(townRoot, rigName)
		return beads.PolecatBeadIDWithPrefix(prefix, rigName, parts[2])
	case len(parts) == 3 && parts[0] == "deacon" && parts[1] == "dogs":
		return beads.DogBeadIDTown(parts[2])
	default:
		return ""
	}
}

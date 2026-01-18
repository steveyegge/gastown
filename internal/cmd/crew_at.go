package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/agent"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/factory"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runCrewAt(cmd *cobra.Command, args []string) error {
	var name string

	// Debug mode: --debug flag or GT_DEBUG env var
	debug := crewDebug || os.Getenv("GT_DEBUG") != ""
	if debug {
		cwd, _ := os.Getwd()
		fmt.Printf("[DEBUG] runCrewAt: args=%v, crewRig=%q, cwd=%q\n", args, crewRig, cwd)
	}

	// Determine crew name: from arg, or auto-detect from cwd
	if len(args) > 0 {
		name = args[0]
		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if rig, crewName, ok := parseRigSlashName(name); ok {
			if crewRig == "" {
				crewRig = rig
			}
			name = crewName
		}
	} else {
		// Try to detect from current directory
		detected, err := detectCrewFromCwd()
		if err != nil {
			// Try to show available crew members if we can detect the rig
			hint := "\n\nUsage: gt crew at <name>"
			if crewRig != "" {
				if mgr, _, mgrErr := getCrewManager(crewRig, ""); mgrErr == nil {
					if members, listErr := mgr.List(); listErr == nil && len(members) > 0 {
						hint = fmt.Sprintf("\n\nAvailable crew in %s:", crewRig)
						for _, m := range members {
							hint += fmt.Sprintf("\n  %s", m.Name)
						}
					}
				}
			}
			return fmt.Errorf("could not detect crew workspace from current directory: %w%s", err, hint)
		}
		name = detected.crewName
		if crewRig == "" {
			crewRig = detected.rigName
		}
		fmt.Printf("Detected crew workspace: %s/%s\n", detected.rigName, name)
	}

	if debug {
		fmt.Printf("[DEBUG] after detection: name=%q, crewRig=%q\n", name, crewRig)
	}

	// Get crew manager (uses factory pattern with proper session setup)
	crewMgr, r, err := getCrewManager(crewRig, crewAgentOverride)
	if err != nil {
		return err
	}

	// Get the crew worker (to check it exists and get path)
	worker, err := crewMgr.Get(name)
	if err != nil {
		if err == crew.ErrCrewNotFound {
			return fmt.Errorf("crew workspace '%s' not found", name)
		}
		return fmt.Errorf("getting crew worker: %w", err)
	}

	// Ensure crew workspace is on default branch (persistent roles should not use feature branches)
	ensureDefaultBranch(worker.ClonePath, fmt.Sprintf("Crew workspace %s/%s", r.Name, name), r.Path)

	// If --no-tmux, just print the path
	if crewNoTmux {
		fmt.Println(worker.ClonePath)
		return nil
	}

	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	sessionName := crewSessionName(r.Name, name)
	if debug {
		fmt.Printf("[DEBUG] sessionName=%q (r.Name=%q, name=%q)\n", sessionName, r.Name, name)
	}

	// Check if session exists using agents interface
	crewID := agent.CrewAddress(r.Name, name)
	agents := factory.Agents()
	hasSession := agents.Exists(crewID)
	if debug {
		fmt.Printf("[DEBUG] hasSession=%v\n", hasSession)
	}

	// Start session if not running (agent resolved automatically, with optional override)
	if !hasSession {
		fmt.Printf("Starting session for %s/%s...\n", r.Name, name)
		opts := []factory.StartOption{factory.WithTopic("start"), factory.WithAgent(crewAgentOverride)}
		if _, err := factory.Start(townRoot, crewID, "", opts...); err != nil {
			return fmt.Errorf("starting crew session: %w", err)
		}
		fmt.Printf("%s Created session for %s/%s\n", style.Bold.Render("✓"), r.Name, name)
	}

	// Don't attach if --detached flag is set
	if crewDetached {
		fmt.Printf("Started %s/%s. Run 'gt crew at %s' to attach.\n", r.Name, name, name)
		return nil
	}

	// Smart attach: switches if inside tmux, attaches if outside
	fmt.Printf("Attaching to %s...\n", sessionName)
	if debug {
		fmt.Printf("[DEBUG] calling agents.Attach(%q)\n", crewID)
	}
	return agents.Attach(crewID)
}

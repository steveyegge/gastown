package cmd

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	crewtui "github.com/steveyegge/gastown/internal/tui/crew"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runCrewAdd(cmd *cobra.Command, args []string) error {
	// Find workspace first (needed for all names)
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	// Launch TUI wizard if --tui flag is set
	if crewTUI {
		currentRig := crewRig
		if currentRig == "" {
			currentRig, _ = inferRigFromCwd(townRoot)
		}
		if currentRig == "" {
			currentRig = "gastown" // fallback
		}
		m := crewtui.NewAddModel(townRoot, currentRig)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("running TUI: %w", err)
		}
		return nil
	}

	// Load rigs config
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
	if err != nil {
		rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	}

	// Determine base rig from --rig flag or first name's rig/name format
	baseRig := crewRig
	if baseRig == "" {
		// Check if first arg has rig/name format
		if parsedRig, _, ok := parseRigSlashName(args[0]); ok {
			baseRig = parsedRig
		}
	}
	if baseRig == "" {
		// Try to infer from cwd
		baseRig, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	// Get rig
	g := git.NewGit(townRoot)
	rigMgr := rig.NewManager(townRoot, rigsConfig, g)
	r, err := rigMgr.GetRig(baseRig)
	if err != nil {
		return fmt.Errorf("rig '%s' not found", baseRig)
	}

	// Create crew manager
	crewGit := git.NewGit(r.Path)
	crewMgr := crew.NewManager(r, crewGit)

	bd := beads.New(beads.ResolveBeadsDir(r.Path))

	// Track results
	var created []string
	var failed []string
	var lastWorker *crew.CrewWorker

	// Process each name
	for _, arg := range args {
		name := arg
		rigName := baseRig

		// Parse rig/name format (e.g., "beads/emma" -> rig=beads, name=emma)
		if parsedRig, crewName, ok := parseRigSlashName(arg); ok {
			// For rig/name format, use that rig (but warn if different from base)
			if parsedRig != baseRig {
				style.PrintWarning("%s: different rig '%s' ignored (use --rig to change)", arg, parsedRig)
			}
			name = crewName
		}

		// Create crew workspace
		fmt.Printf("Creating crew workspace %s in %s...\n", name, rigName)

		worker, err := crewMgr.Add(name, crewBranch)
		if err != nil {
			if err == crew.ErrCrewExists {
				style.PrintWarning("crew workspace '%s' already exists, skipping", name)
				failed = append(failed, name+" (exists)")
				continue
			}
			style.PrintWarning("creating crew workspace '%s': %v", name, err)
			failed = append(failed, name)
			continue
		}

		fmt.Printf("%s Created crew workspace: %s/%s\n",
			style.Bold.Render("✓"), rigName, name)
		fmt.Printf("  Path: %s\n", worker.ClonePath)
		fmt.Printf("  Branch: %s\n", worker.Branch)

		// Create agent bead for the crew worker
		prefix := beads.GetPrefixForRig(townRoot, rigName)
		crewID := beads.CrewBeadIDWithPrefix(prefix, rigName, name)
		if _, err := bd.Show(crewID); err != nil {
			// Agent bead doesn't exist, create it
			fields := &beads.AgentFields{
				RoleType:   "crew",
				Rig:        rigName,
				AgentState: "idle",
			}
			desc := fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", name, rigName)
			if _, err := bd.CreateAgentBead(crewID, desc, fields); err != nil {
				style.PrintWarning("could not create agent bead for %s: %v", name, err)
			} else {
				fmt.Printf("  Agent bead: %s\n", crewID)
			}
		}

		created = append(created, name)
		lastWorker = worker
		fmt.Println()
	}

	// Summary
	if len(created) > 0 {
		fmt.Printf("%s Created %d crew workspace(s): %v\n",
			style.Bold.Render("✓"), len(created), created)
		if lastWorker != nil && len(created) == 1 {
			fmt.Printf("\n%s\n", style.Dim.Render("Start working with: cd "+lastWorker.ClonePath))
		}
	}
	if len(failed) > 0 {
		fmt.Printf("%s Failed to create %d workspace(s): %v\n",
			style.Warning.Render("!"), len(failed), failed)
	}

	// Return error if all failed
	if len(created) == 0 && len(failed) > 0 {
		return fmt.Errorf("failed to create any crew workspaces")
	}

	return nil
}

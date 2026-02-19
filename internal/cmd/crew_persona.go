package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var crewPersonaCmd = &cobra.Command{
	Use:   "persona",
	Short: "Manage crew member personas",
	Long: `Manage behavioral personas for crew members.

Personas are markdown files stored in .personas/ directories and synced to
beads for provenance tracking:
  <rig>/.personas/<name>.md        Rig-level (highest priority)
  <town>/.personas/<name>.md       Town-level (shared across rigs)

When a crew member starts a session, gt prime injects their assigned persona.

Use 'gt crew persona sync' to import .personas/ files into beads storage.`,
	RunE: requireSubcommand,
}

var crewPersonaSetCmd = &cobra.Command{
	Use:   "set <crew> [persona]",
	Short: "Assign a persona to a crew member (omit persona to clear)",
	Long: `Assign a persona to a crew member.

  gt crew persona set alice rust-expert   # Assign rust-expert persona to alice
  gt crew persona set alice               # Clear persona assignment from alice`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runCrewPersonaSet,
}

var crewPersonaShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Show persona content, or list all personas and assignments",
	Long: `Show persona information.

  gt crew persona show              # List all personas and crew assignments
  gt crew persona show rust-expert  # Show content of a specific persona`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runCrewPersonaShow,
}

var crewPersonaSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync .personas/ files to beads storage",
	Long: `Scan .personas/ directories and create or update persona beads.

Only updates beads when file content has changed (hash-gated). Use --force
to overwrite all beads regardless of hash.`,
	Args: cobra.NoArgs,
	RunE: runCrewPersonaSync,
}

var crewPersonaSyncForce bool

func runCrewPersonaSet(cmd *cobra.Command, args []string) error {
	crewName := args[0]

	mgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	bd := beads.New(beads.ResolveBeadsDir(r.Path))
	prefix := beads.GetPrefixForRig(townRoot, r.Name)
	crewID := beads.CrewBeadIDWithPrefix(prefix, r.Name, crewName)

	if len(args) == 1 {
		// Clear persona assignment
		if err := mgr.SetPersona(crewName, ""); err != nil {
			return fmt.Errorf("clearing persona: %w", err)
		}
		emptyPersonaBead := ""
		if updateErr := bd.UpdateAgentDescriptionFields(crewID, beads.AgentFieldUpdates{
			PersonaBead: &emptyPersonaBead,
		}); updateErr != nil {
			// Bead may not exist yet; log but don't fail
			style.PrintWarning("could not clear persona_bead on agent bead %s: %v", crewID, updateErr)
		}
		fmt.Printf("Persona assignment cleared from %q\n", crewName)
		return nil
	}

	// Set persona assignment
	personaName := args[1]
	if err := validatePersonaNameArg(personaName); err != nil {
		return err
	}

	personaBeadID, err := crew.EnsurePersonaBeadExists(townRoot, r.Path, prefix, r.Name, personaName, bd)
	if err != nil {
		return err
	}

	if err := mgr.SetPersona(crewName, personaName); err != nil {
		return fmt.Errorf("setting persona: %w", err)
	}

	if updateErr := bd.UpdateAgentDescriptionFields(crewID, beads.AgentFieldUpdates{
		PersonaBead: &personaBeadID,
	}); updateErr != nil {
		style.PrintWarning("could not set persona_bead on agent bead %s: %v", crewID, updateErr)
	}

	fmt.Printf("Crew member %q assigned persona %q\n", crewName, personaName)
	return nil
}

func runCrewPersonaShow(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}

	bd := beads.New(beads.ResolveBeadsDir(r.Path))
	prefix := beads.GetPrefixForRig(townRoot, r.Name)

	if len(args) == 0 {
		return showAllPersonas(mgr, r.Path, townRoot, prefix, r.Name, bd)
	}

	name := args[0]
	if err := validatePersonaNameArg(name); err != nil {
		return err
	}

	// Try bead first, then fall back to file
	beadID := beads.PersonaBeadID(prefix, r.Name, name)
	content, err := beads.GetPersonaContent(bd, beadID)
	if err != nil {
		return fmt.Errorf("fetching persona bead: %w", err)
	}

	if content == "" {
		// Try town-level bead
		townBeadID := beads.PersonaBeadID(prefix, "", name)
		content, err = beads.GetPersonaContent(bd, townBeadID)
		if err != nil {
			return fmt.Errorf("fetching persona bead: %w", err)
		}
	}

	if content == "" {
		// Fall back to file
		content, _, err = crew.ResolvePersonaFile(townRoot, r.Path, name)
		if err != nil {
			return fmt.Errorf("reading persona file: %w", err)
		}
	}

	if content == "" {
		return fmt.Errorf(
			"persona %q not found (run `gt crew persona sync` to import from .personas/)",
			name,
		)
	}

	fmt.Printf("# Persona: %s\n\n", name)
	fmt.Print(content)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func runCrewPersonaSync(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr, r, err := getCrewManager(crewRig)
	if err != nil {
		return err
	}
	_ = mgr

	bd := beads.New(beads.ResolveBeadsDir(r.Path))
	prefix := beads.GetPrefixForRig(townRoot, r.Name)

	updated, err := crew.SyncPersonasFromFiles(townRoot, r.Path, prefix, r.Name, bd, crewPersonaSyncForce)
	if err != nil {
		return fmt.Errorf("syncing personas: %w", err)
	}

	if len(updated) == 0 {
		fmt.Println("Personas already up to date.")
		return nil
	}

	fmt.Printf("Synced %d persona(s):\n", len(updated))
	for _, name := range updated {
		fmt.Printf("  %s %s\n", style.Success.Render("✓"), name)
	}
	return nil
}

func showAllPersonas(
	mgr *crew.Manager,
	rigPath, townRoot, prefix, rigName string,
	bd *beads.Beads,
) error {
	// List persona beads
	personaBeads, err := beads.ListPersonaBeads(bd)
	if err != nil {
		return fmt.Errorf("listing persona beads: %w", err)
	}

	// Fall back to files if no beads found
	if len(personaBeads) == 0 {
		personas, listErr := crew.ListPersonas(townRoot, rigPath)
		if listErr != nil {
			return fmt.Errorf("listing persona files: %w", listErr)
		}
		if len(personas) == 0 {
			fmt.Println("No personas found.")
			fmt.Printf(
				"\nCreate .personas/<name>.md in your rig or town directory,\n"+
					"then run: gt crew persona sync\n",
			)
			return nil
		}
		fmt.Println("Available personas (from files, not yet synced):")
		for _, p := range personas {
			label := p.Source
			if p.Overrides {
				label = "rig, overrides town"
			}
			fmt.Printf("  %-24s (%s)\n", p.Name, label)
		}
	} else {
		fmt.Println("Available personas:")
		for _, p := range personaBeads {
			fmt.Printf(
				"  %-24s (%s)\n",
				style.Bold.Render(p.Name), p.Source,
			)
		}
	}

	// Show crew assignments
	printCrewPersonaAssignments(mgr, prefix, rigName)
	return nil
}

func printCrewPersonaAssignments(mgr *crew.Manager, prefix, rigName string) {
	workers, err := mgr.List()
	if err != nil || len(workers) == 0 {
		return
	}

	fmt.Println()
	fmt.Println("Crew assignments:")
	for _, w := range workers {
		if w.Persona != "" {
			fmt.Printf(
				"  %-24s %s %s\n",
				w.Name,
				style.Dim.Render("→"),
				w.Persona,
			)
		} else {
			fmt.Printf(
				"  %-24s %s\n",
				w.Name,
				style.Dim.Render("(no persona)"),
			)
		}
	}
}

// validatePersonaNameArg validates a persona name provided as a CLI argument.
func validatePersonaNameArg(name string) error {
	if err := crew.ValidatePersonaName(name); err != nil {
		return err
	}
	return nil
}

func init() {
	crewPersonaSetCmd.Flags().StringVar(&crewRig, "rig", "", "Rig to use")
	crewPersonaShowCmd.Flags().StringVar(&crewRig, "rig", "", "Rig to use")
	crewPersonaSyncCmd.Flags().StringVar(&crewRig, "rig", "", "Rig to use")
	crewPersonaSyncCmd.Flags().BoolVar(
		&crewPersonaSyncForce, "force", false,
		"Overwrite all persona beads regardless of hash",
	)

	crewPersonaCmd.AddCommand(crewPersonaSetCmd)
	crewPersonaCmd.AddCommand(crewPersonaShowCmd)
	crewPersonaCmd.AddCommand(crewPersonaSyncCmd)
	crewCmd.AddCommand(crewPersonaCmd)
}

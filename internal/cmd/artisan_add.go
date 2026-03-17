package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/artisan"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runArtisanAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine rig
	rigName := artisanRig
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	rigPath := filepath.Join(townRoot, rigName)
	layout := rig.NewClassicLayout(rigPath)

	// Validate specialty against configured specialties
	specialties, err := config.LoadSpecialties(layout)
	if err != nil {
		return fmt.Errorf("loading specialties: %w", err)
	}
	if specialties.GetSpecialty(artisanSpecialty) == nil {
		return fmt.Errorf("unknown specialty %q — valid specialties: %s", artisanSpecialty, strings.Join(specialties.Names(), ", "))
	}

	mgr := artisan.NewManager(rigName, layout, townRoot)
	worker, err := mgr.Add(name, artisanSpecialty)
	if err != nil {
		return fmt.Errorf("creating artisan %s: %w", name, err)
	}

	fmt.Printf("Created artisan %s\n", style.Bold.Render(worker.Name))
	fmt.Printf("  Specialty: %s\n", worker.Specialty)
	fmt.Printf("  Rig: %s\n", worker.Rig)
	fmt.Printf("  Path: %s\n", worker.ClonePath)

	return nil
}

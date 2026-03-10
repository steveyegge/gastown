package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/artisan"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

func runArtisanRemove(cmd *cobra.Command, args []string) error {
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

	rigPath := fmt.Sprintf("%s/%s", townRoot, rigName)
	mgr := artisan.NewManager(rigName, rigPath, townRoot)

	for _, name := range args {
		if err := mgr.Remove(name, artisanForce); err != nil {
			return fmt.Errorf("removing artisan %s: %w", name, err)
		}
		fmt.Printf("Removed artisan %s\n", style.Bold.Render(name))
	}

	return nil
}

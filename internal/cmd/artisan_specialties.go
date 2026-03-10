package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var artisanSpecialtiesCmd = &cobra.Command{
	Use:   "specialties [rig]",
	Short: "List configured artisan specialties",
	Long: `List artisan specialties configured for this rig.

Shows built-in defaults merged with any rig-level overrides
from <rig>/conductor/specialties.toml.

Examples:
  gt artisan specialties              # Current rig
  gt artisan specialties gastown      # Specific rig
  gt artisan specialties --json       # JSON output`,
	Args: cobra.MaximumNArgs(1),
	RunE: runArtisanSpecialties,
}

func init() {
	artisanSpecialtiesCmd.Flags().StringVar(&artisanRig, "rig", "", "Rig to query")
	artisanSpecialtiesCmd.Flags().BoolVar(&artisanJSON, "json", false, "Output as JSON")
	artisanCmd.AddCommand(artisanSpecialtiesCmd)
}

func runArtisanSpecialties(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		if artisanRig != "" {
			return fmt.Errorf("cannot specify both positional rig argument and --rig flag")
		}
		artisanRig = args[0]
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	rigName := artisanRig
	if rigName == "" {
		rigName, err = inferRigFromCwd(townRoot)
		if err != nil {
			return fmt.Errorf("could not determine rig (use --rig flag): %w", err)
		}
	}

	rigPath := fmt.Sprintf("%s/%s", townRoot, rigName)
	specialties, err := config.LoadSpecialties(rigPath)
	if err != nil {
		return fmt.Errorf("loading specialties: %w", err)
	}

	if artisanJSON {
		data, err := json.MarshalIndent(specialties.Specialties, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("%s (rig: %s)\n", style.Bold.Render("Artisan Specialties"), rigName)
	for _, s := range specialties.Specialties {
		fmt.Printf("\n  %s\n", style.Bold.Render(s.Name))
		fmt.Printf("    %s\n", s.Description)
		if len(s.FilePatterns) > 0 {
			fmt.Printf("    Files: %v\n", s.FilePatterns)
		}
		if len(s.Labels) > 0 {
			fmt.Printf("    Labels: %v\n", s.Labels)
		}
	}

	return nil
}

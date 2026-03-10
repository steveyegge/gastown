package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/artisan"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

// ArtisanListItem represents an artisan in list output.
type ArtisanListItem struct {
	Name      string `json:"name"`
	Rig       string `json:"rig"`
	Specialty string `json:"specialty"`
	Branch    string `json:"branch"`
	Path      string `json:"path"`
}

func runArtisanList(cmd *cobra.Command, args []string) error {
	// Handle positional rig argument
	if len(args) > 0 {
		if artisanRig != "" {
			return fmt.Errorf("cannot specify both positional rig argument and --rig flag")
		}
		artisanRig = args[0]
	}

	if artisanListAll && artisanRig != "" {
		return fmt.Errorf("cannot use --all with a rig filter")
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return err
	}

	// Determine which rigs to list
	var rigNames []string
	if artisanListAll {
		rigNames, err = getAllRigNames()
		if err != nil {
			return err
		}
	} else {
		rigName := artisanRig
		if rigName == "" {
			rigName, err = inferRigFromCwd(townRoot)
			if err != nil {
				return fmt.Errorf("could not determine rig (use --rig flag or --all): %w", err)
			}
		}
		rigNames = []string{rigName}
	}

	var allItems []ArtisanListItem

	for _, rigName := range rigNames {
		rigPath := fmt.Sprintf("%s/%s", townRoot, rigName)
		mgr := artisan.NewManager(rigName, rigPath, townRoot)

		workers, err := mgr.List()
		if err != nil {
			continue
		}

		for _, w := range workers {
			allItems = append(allItems, ArtisanListItem{
				Name:      w.Name,
				Rig:       w.Rig,
				Specialty: w.Specialty,
				Branch:    w.Branch,
				Path:      w.ClonePath,
			})
		}
	}

	if artisanJSON {
		data, err := json.MarshalIndent(allItems, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	if len(allItems) == 0 {
		fmt.Println("No artisan workspaces found.")
		return nil
	}

	fmt.Println(style.Bold.Render("Artisan Workspaces"))
	for _, item := range allItems {
		fmt.Printf("  %s/%s\n", item.Rig, style.Bold.Render(item.Name))
		fmt.Printf("    Specialty: %s  Branch: %s\n", item.Specialty, item.Branch)
		fmt.Printf("    %s\n", item.Path)
	}

	return nil
}

// getAllRigNames returns all rig names from the town.
func getAllRigNames() ([]string, error) {
	rigs, _, err := getAllRigs()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, r := range rigs {
		names = append(names, r.Name)
	}
	return names, nil
}

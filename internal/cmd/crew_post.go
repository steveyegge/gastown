package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
)

var crewPostClear bool

var crewPostCmd = &cobra.Command{
	Use:   "post <name> [posting]",
	Short: "Set or clear a persistent posting for a crew worker",
	Long: `Set or clear a persistent posting assignment for a crew worker.

A posting is a specialized role that augments the base agent context.
Persistent postings are stored in rig settings (settings/config.json)
and apply every time the worker starts.

Persistent postings block session-level assume (gt posting assume).
To change a persistent posting, clear it first or set a new one.

Examples:
  gt crew post dave dispatcher     # Assign dave the dispatcher posting
  gt crew post emma scout          # Assign emma the scout posting
  gt crew post dave --clear        # Remove dave's persistent posting
  gt crew post dave reviewer --rig gastown  # Specify rig explicitly`,
	Args: func(cmd *cobra.Command, args []string) error {
		if crewPostClear {
			if len(args) != 1 {
				return fmt.Errorf("--clear requires exactly 1 argument: the crew name")
			}
			return nil
		}
		if len(args) != 2 {
			return fmt.Errorf("requires 2 arguments: <name> <posting>")
		}
		return nil
	},
	RunE: runCrewPost,
}

func init() {
	crewPostCmd.Flags().BoolVar(&crewPostClear, "clear", false, "Remove the persistent posting for this worker")
	crewPostCmd.Flags().StringVar(&crewRig, "rig", "", "Rig to use")

	crewCmd.AddCommand(crewPostCmd)
}

func runCrewPost(cmd *cobra.Command, args []string) error {
	crewName := args[0]

	// Resolve rig and verify crew member exists
	mgr, r, err := getCrewManagerForMember(crewRig, crewName)
	if err != nil {
		return err
	}

	// Verify the crew member exists
	if _, err := mgr.Get(crewName); err != nil {
		return fmt.Errorf("crew member %q not found: %w", crewName, err)
	}

	settingsPath := filepath.Join(r.Path, "settings", "config.json")

	// Load existing settings or create new
	settings, err := config.LoadRigSettings(settingsPath)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			settings = config.NewRigSettings()
		} else {
			return fmt.Errorf("loading settings: %w", err)
		}
	}

	if crewPostClear {
		// Clear the posting
		if settings.WorkerPostings == nil {
			fmt.Printf("No posting set for %s\n", crewName)
			return nil
		}
		old, existed := settings.WorkerPostings[crewName]
		if !existed {
			fmt.Printf("No posting set for %s\n", crewName)
			return nil
		}
		delete(settings.WorkerPostings, crewName)
		// Clean up empty map so it doesn't serialize as {}
		if len(settings.WorkerPostings) == 0 {
			settings.WorkerPostings = nil
		}

		if err := config.SaveRigSettings(settingsPath, settings); err != nil {
			return fmt.Errorf("saving settings: %w", err)
		}

		fmt.Printf("%s Cleared posting %q from %s\n",
			style.Success.Render("✓"), old, crewName)
		return nil
	}

	// Set the posting
	postingName := args[1]

	if err := validatePostingName(postingName); err != nil {
		return err
	}

	if settings.WorkerPostings == nil {
		settings.WorkerPostings = make(map[string]string)
	}

	old, hadOld := settings.WorkerPostings[crewName]
	settings.WorkerPostings[crewName] = postingName

	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		return fmt.Errorf("saving settings: %w", err)
	}

	if hadOld && old != postingName {
		fmt.Printf("%s Updated posting for %s: %s → %s\n",
			style.Success.Render("✓"), crewName, old, postingName)
	} else {
		fmt.Printf("%s Set posting for %s: %s\n",
			style.Success.Render("✓"), crewName, postingName)
	}

	// Warn (not block) if the posting template doesn't exist at any level.
	// The posting system is intentionally lenient — the template might be
	// created later — but the warning catches typos.
	townRoot, _ := workspace.FindFromCwd()
	if _, err := templates.LoadPosting(townRoot, r.Path, postingName); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Warning: posting %q has no template defined at any level\n", postingName)
		fmt.Fprintf(os.Stderr, "  Available built-in postings: %v\n", templates.BuiltinPostingNames())
		fmt.Fprintf(os.Stderr, "  Define it at:\n")
		fmt.Fprintf(os.Stderr, "    Rig:  %s/postings/%s.md.tmpl\n", r.Path, postingName)
		if townRoot != "" {
			fmt.Fprintf(os.Stderr, "    Town: %s/postings/%s.md.tmpl\n", townRoot, postingName)
		}
		fmt.Fprintf(os.Stderr, "  Or remove: gt crew post %s --clear\n", crewName)
	}

	return nil
}

package cmd

import (
	"github.com/spf13/cobra"
)

// Artisan command flags
var (
	artisanRig       string
	artisanSpecialty string
	artisanJSON      bool
	artisanForce     bool
	artisanListAll   bool
)

var artisanCmd = &cobra.Command{
	Use:     "artisan",
	GroupID: GroupWorkspace,
	Short:   "Manage artisan workers (specialized long-lived agents)",
	RunE:    requireSubcommand,
	Long: `Manage artisan workers - specialized long-lived agents with domain expertise.

ARTISANS VS POLECATS VS CREW:
  Polecats:  Ephemeral sessions. Witness-managed. Generic workers.
  Crew:      Persistent. User-managed. For human developers.
  Artisans:  Persistent. Conductor-managed. Specialized domain experts.

Artisans are full git clones (like crew) but are assigned a specialty
(frontend, backend, tests, docs, security, etc.) and managed by the
Conductor for automated dispatch.

Commands:
  gt artisan add <name> --specialty <type>   Create artisan workspace
  gt artisan list                             List artisan workspaces
  gt artisan remove <name>                    Remove artisan workspace`,
}

var artisanAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new artisan workspace",
	Long: `Create a new artisan workspace with a specialty assignment.

Each artisan is created at <rig>/artisans/<name>/ with:
- A directory for the workspace (git clone handled separately)
- Mail directory for message delivery
- State file tracking specialty and metadata

The --specialty flag is required and determines the artisan's domain.

Examples:
  gt artisan add frontend-1 --specialty frontend
  gt artisan add backend-1 --specialty backend --rig gastown
  gt artisan add tests-1 --specialty tests
  gt artisan add docs-1 --specialty docs
  gt artisan add security-1 --specialty security`,
	Args: cobra.ExactArgs(1),
	RunE: runArtisanAdd,
}

var artisanListCmd = &cobra.Command{
	Use:   "list [rig]",
	Short: "List artisan workspaces with status",
	Args:  cobra.MaximumNArgs(1),
	Long: `List all artisan workspaces in a rig with their specialty and status.

Examples:
  gt artisan list                     # List in current rig
  gt artisan list gastown             # List in specific rig
  gt artisan list --all               # List in all rigs
  gt artisan list --json              # JSON output`,
	RunE: runArtisanList,
}

var artisanRemoveCmd = &cobra.Command{
	Use:   "remove <name...>",
	Short: "Remove artisan workspace(s)",
	Long: `Remove one or more artisan workspaces from the rig.

Examples:
  gt artisan remove frontend-1
  gt artisan remove frontend-1 backend-1    # Remove multiple
  gt artisan remove frontend-1 --force      # Force remove`,
	Args: cobra.MinimumNArgs(1),
	RunE: runArtisanRemove,
}

func init() {
	artisanAddCmd.Flags().StringVar(&artisanRig, "rig", "", "Rig to create artisan in")
	artisanAddCmd.Flags().StringVar(&artisanSpecialty, "specialty", "", "Artisan specialty (required: frontend, backend, tests, docs, security, etc.)")
	artisanAddCmd.MarkFlagRequired("specialty")

	artisanListCmd.Flags().StringVar(&artisanRig, "rig", "", "Filter by rig name")
	artisanListCmd.Flags().BoolVar(&artisanListAll, "all", false, "List artisans in all rigs")
	artisanListCmd.Flags().BoolVar(&artisanJSON, "json", false, "Output as JSON")

	artisanRemoveCmd.Flags().StringVar(&artisanRig, "rig", "", "Rig to use")
	artisanRemoveCmd.Flags().BoolVar(&artisanForce, "force", false, "Force remove (skip safety checks)")

	artisanCmd.AddCommand(artisanAddCmd)
	artisanCmd.AddCommand(artisanListCmd)
	artisanCmd.AddCommand(artisanRemoveCmd)

	rootCmd.AddCommand(artisanCmd)
}

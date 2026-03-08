package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/gascity"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var gascityValidateJSON bool

var gascityCmd = &cobra.Command{
	Use:     "gascity",
	GroupID: GroupConfig,
	Short:   "Gas City schema and prototype tooling",
	RunE:    requireSubcommand,
	Long: `Gas City prototype tooling.

Current commands validate the emerging declarative role schema used by the
Gas City role parser prototype.`,
}

var gascityRoleCmd = &cobra.Command{
	Use:   "role",
	Short: "Work with Gas City role specs",
	RunE:  requireSubcommand,
}

var gascityRoleValidateCmd = &cobra.Command{
	Use:   "validate <file>",
	Short: "Validate a Gas City role spec",
	Long: `Validate a Gas City role spec TOML file.

The validator loads built-in provider presets and, when run from inside a Gas
Town workspace, also loads the current town's custom agent registry so custom
providers can be validated too.

Examples:
  gt gascity role validate docs/examples/gascity-role.example.toml
  gt gascity role validate role.toml --json`,
	Args: cobra.ExactArgs(1),
	RunE: runGascityRoleValidate,
}

func init() {
	gascityRoleValidateCmd.Flags().BoolVar(&gascityValidateJSON, "json", false, "Output normalized spec as JSON")
	gascityRoleCmd.AddCommand(gascityRoleValidateCmd)
	gascityCmd.AddCommand(gascityRoleCmd)
	rootCmd.AddCommand(gascityCmd)
}

func runGascityRoleValidate(cmd *cobra.Command, args []string) error {
	if townRoot, err := workspace.FindFromCwd(); err == nil && townRoot != "" {
		_ = config.LoadAgentRegistry(config.DefaultAgentRegistryPath(townRoot))
	}

	spec, err := gascity.LoadRoleSpec(args[0])
	if err != nil {
		return err
	}

	if gascityValidateJSON {
		out, err := json.MarshalIndent(spec, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("%s Valid Gas City role spec\n", style.Bold.Render("✓"))
	fmt.Printf("  File: %s\n", args[0])
	fmt.Printf("  Role: %s\n", spec.Role)
	fmt.Printf("  Scope: %s\n", spec.Scope)
	fmt.Printf("  Provider: %s\n", spec.Provider)
	fmt.Printf("  Ready Strategy: %s\n", spec.Capabilities.ReadyStrategy)
	fmt.Printf("  Hooks: %t\n", spec.Capabilities.Hooks)
	fmt.Printf("  Resume: %t\n", spec.Capabilities.Resume)
	fmt.Printf("  Fork Session: %t\n", spec.Capabilities.ForkSession)
	fmt.Printf("  Exec: %t\n", spec.Capabilities.Exec)
	return nil
}

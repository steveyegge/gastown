package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/templates"
	"github.com/steveyegge/gastown/internal/workspace"
)

var overrideCmd = &cobra.Command{
	Use:     "override",
	GroupID: GroupAgents,
	Short:   "Manage role prompt overrides",
	Long: `Manage local overrides for role prompts.

Overrides allow you to customize role prompts (mayor, witness, refinery, etc.)
without modifying the gastown source code. Overrides are stored in your town
root and tracked in git.

Override path: $GT_TOWN_ROOT/.gt/overrides/{role}.md.tmpl

Examples:
  gt override list                    # List all active overrides
  gt override show mayor              # Show mayor override content
  gt override edit mayor              # Edit mayor override in $EDITOR
  gt override create mayor            # Create new mayor override from template
  gt override delete mayor            # Remove mayor override (use embedded)
  gt override diff mayor              # Compare override with embedded template`,
	RunE: runOverrideList,
}

var overrideListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active overrides",
	RunE:  runOverrideList,
}

var overrideShowCmd = &cobra.Command{
	Use:   "show <role>",
	Short: "Show override content",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideShow,
}

var overrideEditCmd = &cobra.Command{
	Use:   "edit <role>",
	Short: "Edit override in $EDITOR",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideEdit,
}

var overrideCreateCmd = &cobra.Command{
	Use:   "create <role>",
	Short: "Create new override from embedded template",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideCreate,
}

var overrideDeleteCmd = &cobra.Command{
	Use:   "delete <role>",
	Short: "Remove override (revert to embedded)",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideDelete,
}

var overrideDiffCmd = &cobra.Command{
	Use:   "diff <role>",
	Short: "Compare override with embedded template",
	Args:  cobra.ExactArgs(1),
	RunE:  runOverrideDiff,
}

func init() {
	overrideCmd.AddCommand(overrideListCmd)
	overrideCmd.AddCommand(overrideShowCmd)
	overrideCmd.AddCommand(overrideEditCmd)
	overrideCmd.AddCommand(overrideCreateCmd)
	overrideCmd.AddCommand(overrideDeleteCmd)
	overrideCmd.AddCommand(overrideDiffCmd)
	rootCmd.AddCommand(overrideCmd)
}

func runOverrideList(cmd *cobra.Command, args []string) error {
	overrides, err := templates.ListOverrides()
	if err != nil {
		return fmt.Errorf("listing overrides: %w", err)
	}

	if len(overrides) == 0 {
		fmt.Println(style.Warning.Render("No active overrides. Using embedded templates."))
		return nil
	}

	fmt.Println(style.Bold.Render("Active Overrides:"))
	fmt.Println()

	for _, role := range overrides {
		fmt.Printf("  • %s\n", style.Info.Render(role))
	}

	fmt.Println()
	fmt.Println("Edit with: gt override edit <role>")
	fmt.Println("Delete with: gt override delete <role>")

	return nil
}

func runOverrideShow(cmd *cobra.Command, args []string) error {
	role := args[0]
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s (valid: %s)", role, strings.Join(validRoles(), ", "))
	}

	if !templates.HasOverride(role) {
		fmt.Printf("No override for %s. Using embedded template.\n", style.Info.Render(role))
		return nil
	}

	overridePath := getOverridePath(role)
	content, err := os.ReadFile(overridePath)
	if err != nil {
		return fmt.Errorf("reading override: %w", err)
	}

	fmt.Println(string(content))
	return nil
}

func runOverrideEdit(cmd *cobra.Command, args []string) error {
	role := args[0]
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s (valid: %s)", role, strings.Join(validRoles(), ", "))
	}

	overridePath := getOverridePath(role)

	// Check if override exists
	if _, err := os.Stat(overridePath); os.IsNotExist(err) {
		fmt.Printf("No override for %s. Create it first:\n", style.Info.Render(role))
		fmt.Printf("  gt override create %s\n", role)
		return nil
	}

	// Get editor from environment
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vim"
	}

	// Run editor
	cmdExec := exec.Command(editor, overridePath)
	cmdExec.Stdin = os.Stdin
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}

	fmt.Printf("Override for %s updated.\n", style.Info.Render(role))
	fmt.Println("Restart the agent to apply changes:")
	fmt.Printf("  gt %s restart\n", role)

	return nil
}

func runOverrideCreate(cmd *cobra.Command, args []string) error {
	role := args[0]
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s (valid: %s)", role, strings.Join(validRoles(), ", "))
	}

	overridePath := getOverridePath(role)

	// Check if already exists
	if _, err := os.Stat(overridePath); err == nil {
		fmt.Printf("Override for %s already exists:\n", style.Info.Render(role))
		fmt.Printf("  %s\n", overridePath)
		fmt.Println("Edit with: gt override edit", role)
		return nil
	}

	// Get town root
	townRoot, _, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Create override directory
	overrideDir := filepath.Join(townRoot, ".gt", "overrides")
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		return fmt.Errorf("creating override directory: %w", err)
	}

	// Create override from embedded template
	tmpl, err := templates.New()
	if err != nil {
		return fmt.Errorf("loading templates: %w", err)
	}

	// Get current role data for rendering
	roleData := templates.RoleData{
		Role:     role,
		TownRoot: townRoot,
		TownName: filepath.Base(townRoot),
		WorkDir:  townRoot,
	}

	content, err := tmpl.RenderRole(role, roleData)
	if err != nil {
		return fmt.Errorf("rendering template: %w", err)
	}

	// Write override
	if err := os.WriteFile(overridePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing override: %w", err)
	}

	fmt.Printf("Created override for %s:\n", style.Info.Render(role))
	fmt.Printf("  %s\n", overridePath)
	fmt.Println()
	fmt.Println("Edit with: gt override edit", role)
	fmt.Println("Restart agent: gt", role, "restart")
	fmt.Println()
	fmt.Println(style.Warning.Render("Remember to commit and push to backup your override:"))
	fmt.Println("  cd ~/gt")
	fmt.Println("  git add .gt/overrides/")
	fmt.Println("  git commit -m \"Add", role, "override\"")
	fmt.Println("  git push")

	return nil
}

func runOverrideDelete(cmd *cobra.Command, args []string) error {
	role := args[0]
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s (valid: %s)", role, strings.Join(validRoles(), ", "))
	}

	if !templates.HasOverride(role) {
		fmt.Printf("No override for %s.\n", style.Info.Render(role))
		return nil
	}

	overridePath := getOverridePath(role)
	if err := os.Remove(overridePath); err != nil {
		return fmt.Errorf("deleting override: %w", err)
	}

	fmt.Printf("Deleted override for %s.\n", style.Info.Render(role))
	fmt.Println("Agent will now use embedded template.")
	fmt.Println("Restart to apply: gt", role, "restart")

	return nil
}

func runOverrideDiff(cmd *cobra.Command, args []string) error {
	role := args[0]
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s (valid: %s)", role, strings.Join(validRoles(), ", "))
	}

	if !templates.HasOverride(role) {
		fmt.Printf("No override for %s. Using embedded template.\n", style.Info.Render(role))
		return nil
	}

	// Read override
	overridePath := getOverridePath(role)
	overrideContent, err := os.ReadFile(overridePath)
	if err != nil {
		return fmt.Errorf("reading override: %w", err)
	}

	// Get embedded template (simplified - just show path)
	fmt.Printf("Override for %s:\n", style.Info.Render(role))
	fmt.Printf("  Path: %s\n", overridePath)
	fmt.Printf("  Lines: %d\n", len(strings.Split(string(overrideContent), "\n")))
	fmt.Println()
	fmt.Println("To see full diff, compare with embedded template:")
	fmt.Printf("  diff %s\n", overridePath)
	fmt.Println("    <embedded template in sfgastown source>")

	return nil
}

func isValidRole(role string) bool {
	valid := validRoles()
	for _, v := range valid {
		if v == role {
			return true
		}
	}
	return false
}

func validRoles() []string {
	return []string{"mayor", "witness", "refinery", "polecat", "crew", "deacon", "boot"}
}

func getOverridePath(role string) string {
	// Get town root
	townRoot, _, err := workspace.FindFromCwdWithFallback()
	if err != nil {
		return ""
	}
	return filepath.Join(townRoot, ".gt", "overrides", role+".md.tmpl")
}

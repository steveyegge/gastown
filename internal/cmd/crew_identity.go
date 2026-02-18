package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/rig"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
)

var crewIdentityTown bool
var crewIdentityFile string

var crewIdentityCmd = &cobra.Command{
	Use:   "identity",
	Short: "Manage crew member identities (persistent personalities)",
	Long: `Manage identity files that give crew members persistent personalities.

Identity files are plain markdown stored in layered directories:
  <rig>/identities/<name>.md        Rig-level (highest priority)
  <town>/identities/<name>.md       Town-level (shared fallback)

When a crew member starts a session, gt prime looks for a matching
identity file and injects it into the agent's context.`,
	RunE: requireSubcommand,
}

var crewIdentityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available identities and assignments",
	RunE:  runCrewIdentityList,
}

var crewIdentityShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show identity file content",
	Args:  cobra.ExactArgs(1),
	RunE:  runCrewIdentityShow,
}

var crewIdentityCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new identity file",
	Long: `Create a new identity file for crew members.

Identity files are plain markdown that define personality, expertise,
and communication style. By default, creates in the rig's identities
directory. Use --town to create at town level (shared across rigs).

Content sources (in priority order):
  --file <path>    Read content from a file
  stdin            Pipe content via stdin (detected automatically)
  $EDITOR          Opens your editor if neither --file nor stdin`,
	Args: cobra.ExactArgs(1),
	RunE: runCrewIdentityCreate,
}

var crewIdentityApplyCmd = &cobra.Command{
	Use:   "apply <crew> <identity>",
	Short: "Assign an identity to a crew member",
	Long: `Assign a specific identity to a crew member.

The crew member will use this identity instead of name-based lookup.
The identity file must exist at rig or town level.`,
	Args: cobra.ExactArgs(2),
	RunE: runCrewIdentityApply,
}

var crewIdentityRemoveCmd = &cobra.Command{
	Use:   "remove <crew>",
	Short: "Remove identity assignment from crew member",
	Long: `Remove the explicit identity assignment from a crew member.

The crew member reverts to name-based identity lookup.`,
	Args: cobra.ExactArgs(1),
	RunE: runCrewIdentityRemove,
}

func runCrewIdentityList(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	_, r, err := getCrewManagerForIdentity()
	if err != nil {
		return err
	}

	identities, err := crew.ListIdentities(townRoot, r.Path)
	if err != nil {
		return fmt.Errorf("listing identities: %w", err)
	}

	if len(identities) == 0 {
		fmt.Println("No identity files found.")
		fmt.Printf("\nCreate one with: gt crew identity create <name>\n")
		fmt.Printf(
			"Locations: %s/identities/ or %s/identities/\n",
			r.Path, townRoot,
		)
		return nil
	}

	fmt.Println("Available identities:")
	for _, id := range identities {
		label := id.Source
		if id.Overrides {
			label = "rig, overrides town"
		}
		fmt.Printf("  %-20s (%s)\n", id.Name, label)
	}

	printCrewAssignments(identities)

	return nil
}

func printCrewAssignments(identities []crew.IdentityInfo) {
	mgr, _, _ := getCrewManagerForIdentity()
	if mgr == nil {
		return
	}

	workers, err := mgr.List()
	if err != nil || len(workers) == 0 {
		return
	}

	identitySet := make(map[string]bool, len(identities))
	for _, id := range identities {
		identitySet[id.Name] = true
	}

	fmt.Println()
	fmt.Println("Crew assignments:")
	for _, w := range workers {
		if w.Identity != "" {
			fmt.Printf(
				"  %-20s \u2192 %s\n", w.Name, w.Identity,
			)
		} else if identitySet[w.Name] {
			fmt.Printf(
				"  %-20s \u2192 %s %s\n",
				w.Name, w.Name, style.Dim.Render("(default)"),
			)
		} else {
			fmt.Printf(
				"  %-20s \u2192 %s\n",
				w.Name, style.Dim.Render("(no identity)"),
			)
		}
	}
}

func runCrewIdentityShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	_, r, err := getCrewManagerForIdentity()
	if err != nil {
		return err
	}

	content, source, err := crew.ResolveIdentityFile(
		townRoot, r.Path, name,
	)
	if err != nil {
		return fmt.Errorf("resolving identity: %w", err)
	}
	if content == "" {
		return fmt.Errorf(
			"identity %q not found in rig or town identities directories",
			name,
		)
	}

	fmt.Printf("# Identity: %s (from %s)\n\n", name, source)
	fmt.Print(content)
	if !strings.HasSuffix(content, "\n") {
		fmt.Println()
	}
	return nil
}

func runCrewIdentityCreate(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := crew.ValidateIdentityName(name); err != nil {
		return err
	}

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	_, r, err := getCrewManagerForIdentity()
	if err != nil {
		return err
	}

	var targetDir string
	if crewIdentityTown {
		targetDir = filepath.Join(townRoot, "identities")
	} else {
		targetDir = filepath.Join(r.Path, "identities")
	}

	targetFile := filepath.Join(targetDir, name+".md")

	if _, err := os.Stat(targetFile); err == nil {
		return fmt.Errorf(
			"identity %q already exists at %s", name, targetFile,
		)
	}

	content, err := readIdentityContent(name)
	if err != nil {
		return fmt.Errorf("reading content: %w", err)
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("creating identities directory: %w", err)
	}
	if err := os.WriteFile(targetFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing identity file: %w", err)
	}

	level := "rig"
	if crewIdentityTown {
		level = "town"
	}
	fmt.Printf(
		"Created identity %q at %s level: %s\n",
		name, level, targetFile,
	)
	return nil
}

func runCrewIdentityApply(cmd *cobra.Command, args []string) error {
	crewName := args[0]
	identityName := args[1]

	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("not in a Gas Town workspace: %w", err)
	}

	mgr, r, err := getCrewManagerForIdentity()
	if err != nil {
		return err
	}

	content, _, err := crew.ResolveIdentityFile(
		townRoot, r.Path, identityName,
	)
	if err != nil {
		return fmt.Errorf("resolving identity: %w", err)
	}
	if content == "" {
		return fmt.Errorf(
			"identity %q not found in rig or town identities directories",
			identityName,
		)
	}

	if err := mgr.SetIdentity(crewName, identityName); err != nil {
		return fmt.Errorf("setting identity: %w", err)
	}

	fmt.Printf(
		"Crew member %q will use identity %q\n",
		crewName, identityName,
	)
	return nil
}

func runCrewIdentityRemove(cmd *cobra.Command, args []string) error {
	crewName := args[0]

	mgr, _, err := getCrewManagerForIdentity()
	if err != nil {
		return err
	}

	if err := mgr.ClearIdentity(crewName); err != nil {
		return fmt.Errorf("clearing identity: %w", err)
	}

	fmt.Printf(
		"Identity assignment removed from %q"+
			" (reverts to name-based lookup)\n",
		crewName,
	)
	return nil
}

// readIdentityContent reads identity content from --file, stdin,
// or $EDITOR.
func readIdentityContent(name string) (string, error) {
	if crewIdentityFile != "" {
		data, err := os.ReadFile(crewIdentityFile)
		if err != nil {
			return "", fmt.Errorf(
				"reading file %s: %w", crewIdentityFile, err,
			)
		}
		return string(data), nil
	}

	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}

	return openEditorForIdentity(name)
}

// openEditorForIdentity opens $EDITOR with a template for a new
// identity.
func openEditorForIdentity(name string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	tmpFile, err := os.CreateTemp("", "gt-identity-*.md")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	template := fmt.Sprintf(
		"# %s\n\nYou are %s.\n\n## Expertise\n\n- \n\n"+
			"## Style\n\n- \n",
		name, name,
	)
	if _, err := tmpFile.WriteString(template); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("writing template: %w", err)
	}
	tmpFile.Close()

	editorCmd := exec.Command(editor, tmpPath)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr
	if err := editorCmd.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("reading edited file: %w", err)
	}

	content := string(data)
	if strings.TrimSpace(content) == "" || content == template {
		return "", fmt.Errorf(
			"identity content is empty or unchanged; aborting",
		)
	}

	return content, nil
}

// getCrewManagerForIdentity returns a crew manager using the
// crewRig flag or cwd inference.
func getCrewManagerForIdentity() (*crew.Manager, *rig.Rig, error) {
	return getCrewManager(crewRig)
}

func init() {
	crewIdentityListCmd.Flags().StringVar(
		&crewRig, "rig", "", "Rig to use",
	)
	crewIdentityShowCmd.Flags().StringVar(
		&crewRig, "rig", "", "Rig to use",
	)
	crewIdentityCreateCmd.Flags().StringVar(
		&crewRig, "rig", "", "Rig to use",
	)
	crewIdentityCreateCmd.Flags().BoolVar(
		&crewIdentityTown, "town", false,
		"Create at town level (shared across rigs)",
	)
	crewIdentityCreateCmd.Flags().StringVar(
		&crewIdentityFile, "file", "", "Read content from file",
	)
	crewIdentityApplyCmd.Flags().StringVar(
		&crewRig, "rig", "", "Rig to use",
	)
	crewIdentityRemoveCmd.Flags().StringVar(
		&crewRig, "rig", "", "Rig to use",
	)

	crewIdentityCmd.AddCommand(crewIdentityListCmd)
	crewIdentityCmd.AddCommand(crewIdentityShowCmd)
	crewIdentityCmd.AddCommand(crewIdentityCreateCmd)
	crewIdentityCmd.AddCommand(crewIdentityApplyCmd)
	crewIdentityCmd.AddCommand(crewIdentityRemoveCmd)
	crewCmd.AddCommand(crewIdentityCmd)
}

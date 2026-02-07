package cmd

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/formula"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Formula command flags
var (
	formulaListJSON    bool
	formulaShowJSON    bool
	formulaRunPR       int
	formulaRunRig      string
	formulaRunDryRun   bool
	formulaCreateType  string
	formulaModifyRig   string
	formulaModifyTown  string
	formulaResetRig    string
	formulaUpdateApply bool
)

var formulaCmd = &cobra.Command{
	Use:     "formula",
	Aliases: []string{"formulas"},
	GroupID: GroupWork,
	Short:   "Manage workflow formulas",
	RunE:    requireSubcommand,
	Long: `Manage workflow formulas - reusable molecule templates.

Formulas are TOML/JSON files that define workflows with steps, variables,
and composition rules. They can be "poured" to create molecules or "wisped"
for ephemeral patrol cycles.

Commands:
  list    List available formulas from all search paths
  show    Display formula details (steps, variables, composition)
  run     Execute a formula (pour and dispatch)
  create  Create a new formula template

Search paths (in order):
  1. .beads/formulas/ (project)
  2. ~/.beads/formulas/ (user)
  3. $GT_ROOT/.beads/formulas/ (orchestrator)

Examples:
  gt formula list                    # List all formulas
  gt formula show shiny              # Show formula details
  gt formula run shiny --pr=123      # Run formula on PR #123
  gt formula create my-workflow      # Create new formula template`,
}

var formulaListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available formulas",
	Long: `List available formulas from all search paths.

Searches for formula files (.formula.toml, .formula.json) in:
  1. .beads/formulas/ (project)
  2. ~/.beads/formulas/ (user)
  3. $GT_ROOT/.beads/formulas/ (orchestrator)

Examples:
  gt formula list            # List all formulas
  gt formula list --json     # JSON output`,
	RunE: runFormulaList,
}

var formulaShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Display formula details",
	Long: `Display detailed information about a formula.

Shows:
  - Formula metadata (name, type, description)
  - Variables with defaults and constraints
  - Steps with dependencies
  - Composition rules (extends, aspects)

Examples:
  gt formula show shiny
  gt formula show rule-of-five --json`,
	Args: cobra.ExactArgs(1),
	RunE: runFormulaShow,
}

var formulaRunCmd = &cobra.Command{
	Use:   "run [name]",
	Short: "Execute a formula",
	Long: `Execute a formula by pouring it and dispatching work.

This command:
  1. Looks up the formula by name (or uses default from rig config)
  2. Pours it to create a molecule (or uses existing proto)
  3. Dispatches the molecule to available workers

For PR-based workflows, use --pr to specify the GitHub PR number.

If no formula name is provided, uses the default formula configured in
the rig's settings/config.json under workflow.default_formula.

Options:
  --pr=N      Run formula on GitHub PR #N
  --rig=NAME  Target specific rig (default: current or gastown)
  --dry-run   Show what would happen without executing

Examples:
  gt formula run shiny                    # Run formula in current rig
  gt formula run                          # Run default formula from rig config
  gt formula run shiny --pr=123           # Run on PR #123
  gt formula run security-audit --rig=beads  # Run in specific rig
  gt formula run release --dry-run        # Preview execution`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFormulaRun,
}

var formulaCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new formula template",
	Long: `Create a new formula template file.

Creates a starter formula file in .beads/formulas/ with the given name.
The template includes common sections that you can customize.

Formula types:
  task      Single-step task formula (default)
  workflow  Multi-step workflow with dependencies
  patrol    Repeating patrol cycle (for wisps)

Examples:
  gt formula create my-task                  # Create task formula
  gt formula create my-workflow --type=workflow
  gt formula create nightly-check --type=patrol`,
	Args: cobra.ExactArgs(1),
	RunE: runFormulaCreate,
}

var formulaModifyCmd = &cobra.Command{
	Use:   "modify <name>",
	Short: "Copy an embedded formula for customization",
	Long: `Copy an embedded formula to a local path for customization.

This copies the embedded formula to your town or rig's .beads/formulas/
directory where you can modify it. Local formulas take precedence over
embedded ones in the resolution order.

Resolution order (most specific wins):
  1. Rig:      <rig>/.beads/formulas/     (project-specific)
  2. Town:     $GT_ROOT/.beads/formulas/  (user customizations)
  3. Embedded: (compiled in binary)        (defaults)

Examples:
  gt formula modify shiny                # Copy to town level
  gt formula modify shiny --rig=gastown  # Copy to rig level
  gt formula modify shiny --town=/path   # Copy to explicit town path`,
	Args: cobra.ExactArgs(1),
	RunE: runFormulaModify,
}

var formulaDiffCmd = &cobra.Command{
	Use:   "diff [name]",
	Short: "Show formula overrides and differences",
	Long: `Show formula overrides and their differences from embedded versions.

Without arguments, shows a summary map of all formula overrides across
your town and rigs.

With a formula name, shows detailed side-by-side diffs between each
resolution level (embedded -> town -> rig).

Examples:
  gt formula diff                    # Summary of all overrides
  gt formula diff shiny              # Detailed diff for shiny formula`,
	Args: cobra.MaximumNArgs(1),
	RunE: runFormulaDiff,
}

var formulaResetCmd = &cobra.Command{
	Use:   "reset <name>",
	Short: "Remove a formula override",
	Long: `Remove a local formula override, restoring the embedded version.

By default, removes the override from the town level. Use --rig to
remove from a specific rig instead.

If both town and rig overrides exist, you must specify which to remove
using the --rig flag, or remove both separately.

Examples:
  gt formula reset shiny             # Remove town-level override
  gt formula reset shiny --rig=myproject  # Remove rig-level override`,
	Args: cobra.ExactArgs(1),
	RunE: runFormulaReset,
}

var formulaUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Agent-assisted merge of updated embedded formula into override",
	Long: `Update a formula override when the embedded version has changed.

Detects if the embedded formula has been updated since the override was created,
then invokes an AI agent to merge the changes while preserving your customizations.

The agent is detected from:
  1. $GT_DEFAULT_AGENT environment variable
  2. Town/rig config (default_agent setting)
  3. First available agent on PATH (claude, opencode, etc.)

Without --apply, the merged result is printed to stdout for review.
With --apply, the override file is updated (a .bak backup is created first).

Examples:
  gt formula update shiny                 # Preview merged result
  gt formula update shiny --apply         # Apply merged result to override`,
	Args: cobra.ExactArgs(1),
	RunE: runFormulaUpdate,
}

func init() {
	// List flags
	formulaListCmd.Flags().BoolVar(&formulaListJSON, "json", false, "Output as JSON")

	// Show flags
	formulaShowCmd.Flags().BoolVar(&formulaShowJSON, "json", false, "Output as JSON")

	// Run flags
	formulaRunCmd.Flags().IntVar(&formulaRunPR, "pr", 0, "GitHub PR number to run formula on")
	formulaRunCmd.Flags().StringVar(&formulaRunRig, "rig", "", "Target rig (default: current or gastown)")
	formulaRunCmd.Flags().BoolVar(&formulaRunDryRun, "dry-run", false, "Preview execution without running")

	// Create flags
	formulaCreateCmd.Flags().StringVar(&formulaCreateType, "type", "task", "Formula type: task, workflow, or patrol")

	// Modify flags
	formulaModifyCmd.Flags().StringVar(&formulaModifyRig, "rig", "", "Copy to rig level (<rig>/.beads/formulas/)")
	formulaModifyCmd.Flags().StringVar(&formulaModifyTown, "town", "", "Explicit town path override")

	// Reset flags
	formulaResetCmd.Flags().StringVar(&formulaResetRig, "rig", "", "Remove from specific rig level")

	// Update flags
	formulaUpdateCmd.Flags().BoolVar(&formulaUpdateApply, "apply", false, "Write merged result directly to override file (creates .bak backup)")

	// Add subcommands
	formulaCmd.AddCommand(formulaListCmd)
	formulaCmd.AddCommand(formulaShowCmd)
	formulaCmd.AddCommand(formulaRunCmd)
	formulaCmd.AddCommand(formulaCreateCmd)
	formulaCmd.AddCommand(formulaModifyCmd)
	formulaCmd.AddCommand(formulaDiffCmd)
	formulaCmd.AddCommand(formulaResetCmd)
	formulaCmd.AddCommand(formulaUpdateCmd)

	rootCmd.AddCommand(formulaCmd)
}

// runFormulaList shows all available formulas with override status
func runFormulaList(cmd *cobra.Command, args []string) error {
	// If JSON requested, delegate to bd for compatibility
	if formulaListJSON {
		bdArgs := []string{"formula", "list", "--json"}
		bdCmd := exec.Command("bd", bdArgs...)
		bdCmd.Stdout = os.Stdout
		bdCmd.Stderr = os.Stderr
		return bdCmd.Run()
	}

	// Get town root for override scanning
	townRoot, townErr := findTownRoot()

	// Get embedded formula names
	embeddedNames, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		return fmt.Errorf("reading embedded formulas: %w", err)
	}

	// Scan for overrides
	var overrides []FormulaOverride
	var customFormulas []FormulaOverride
	if townErr == nil {
		overrides = scanAllFormulaOverrides(townRoot)
		customFormulas = findCustomFormulas(townRoot, embeddedNames)
	}

	// Build override lookup map
	overrideMap := make(map[string][]FormulaOverride)
	for _, o := range overrides {
		overrideMap[o.Name] = append(overrideMap[o.Name], o)
	}

	// Print embedded formulas
	fmt.Printf("Embedded Formulas (%d)\n", len(embeddedNames))
	fmt.Printf("──────────────────────\n")

	for _, name := range embeddedNames {
		ovrs, hasOverride := overrideMap[name]
		if !hasOverride {
			fmt.Printf("  %s\n", name)
		} else {
			// Determine which level has the override
			var indicator string
			for _, o := range ovrs {
				if o.Level == "rig" {
					indicator = fmt.Sprintf("◄ rig override (%s)", o.RigName)
					break // Rig takes precedence
				} else if o.Level == "town" {
					indicator = "◄ town override"
				}
			}
			fmt.Printf("  %-28s %s\n", name, style.Dim.Render(indicator))
		}
	}

	// Print custom formulas if any
	if len(customFormulas) > 0 {
		fmt.Printf("\nCustom Formulas (%d)\n", len(customFormulas))
		fmt.Printf("───────────────────\n")

		for _, cf := range customFormulas {
			var location string
			if cf.Level == "rig" {
				location = fmt.Sprintf("(rig: %s)", cf.RigName)
			} else {
				location = "(town)"
			}
			fmt.Printf("  %-28s %s\n", cf.Name, style.Dim.Render(location))
		}
	}

	fmt.Printf("\nRun 'gt formula diff' to see differences.\n")
	fmt.Printf("Run 'gt formula modify <name>' to customize a formula.\n")

	return nil
}

// runFormulaShow displays formula details
// For embedded formulas, handles display directly; otherwise delegates to bd
func runFormulaShow(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	// Check if formula exists in embedded resources (as fallback)
	loc, err := findFormulaWithSource(formulaName)
	if err != nil {
		return err
	}

	// If formula is on disk, delegate to bd for full parsing support
	if !loc.IsEmbedded() {
		bdArgs := []string{"formula", "show", formulaName}
		if formulaShowJSON {
			bdArgs = append(bdArgs, "--json")
		}
		bdCmd := exec.Command("bd", bdArgs...)
		bdCmd.Stdout = os.Stdout
		bdCmd.Stderr = os.Stderr
		return bdCmd.Run()
	}

	// Handle embedded formula display
	return showEmbeddedFormula(formulaName, formulaShowJSON)
}

// showEmbeddedFormula displays an embedded formula's details
func showEmbeddedFormula(name string, jsonOutput bool) error {
	content, err := formula.GetEmbeddedFormula(name)
	if err != nil {
		return err
	}

	if jsonOutput {
		// Parse and output as JSON
		f, err := parseFormulaContent(content)
		if err != nil {
			return err
		}
		// Simple JSON output
		fmt.Printf(`{"name":%q,"description":%q,"type":%q,"source":"embedded"}`,
			f.Name, f.Description, f.Type)
		fmt.Println()
		return nil
	}

	// Human-readable output
	f, err := parseFormulaContent(content)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", style.Bold.Render(f.Name))
	fmt.Printf("  Source: %s\n", style.Dim.Render("embedded"))
	if f.Type != "" {
		fmt.Printf("  Type: %s\n", f.Type)
	}
	if f.Description != "" {
		fmt.Printf("\n%s\n", f.Description)
	}

	// Show legs for convoy formulas
	if len(f.Legs) > 0 {
		fmt.Printf("\n%s\n", style.Bold.Render("Legs:"))
		for _, leg := range f.Legs {
			fmt.Printf("  • %s: %s\n", leg.ID, leg.Title)
			if leg.Focus != "" {
				fmt.Printf("    Focus: %s\n", style.Dim.Render(leg.Focus))
			}
		}
	}

	// Show synthesis if present
	if f.Synthesis != nil {
		fmt.Printf("\n%s\n", style.Bold.Render("Synthesis:"))
		fmt.Printf("  %s\n", f.Synthesis.Title)
	}

	return nil
}

// parseFormulaContent parses formula content bytes into formulaData
func parseFormulaContent(data []byte) (*formulaData, error) {
	f := &formulaData{
		Prompts: make(map[string]string),
	}

	content := string(data)

	// Parse formula name
	if match := extractTOMLValue(content, "formula"); match != "" {
		f.Name = match
	}

	// Parse description
	if match := extractTOMLMultiline(content, "description"); match != "" {
		f.Description = match
	}

	// Parse type
	if match := extractTOMLValue(content, "type"); match != "" {
		f.Type = match
	}

	// Parse legs (convoy formulas)
	f.Legs = extractLegs(content)

	// Parse synthesis
	f.Synthesis = extractSynthesis(content)

	// Parse prompts
	f.Prompts = extractPrompts(content)

	// Parse output config
	f.Output = extractOutput(content)

	return f, nil
}

// runFormulaRun executes a formula by spawning a convoy of polecats.
// For convoy-type formulas, it creates a convoy bead, creates leg beads,
// and slings each leg to a separate polecat with leg-specific prompts.
func runFormulaRun(cmd *cobra.Command, args []string) error {
	// Determine target rig first (needed for default formula lookup)
	targetRig := formulaRunRig
	var rigPath string
	if targetRig == "" {
		// Try to detect from current directory
		townRoot, err := workspace.FindFromCwd()
		if err == nil && townRoot != "" {
			rigName, r, rigErr := findCurrentRig(townRoot)
			if rigErr == nil && rigName != "" {
				targetRig = rigName
				if r != nil {
					rigPath = r.Path
				}
			}
			// If we still don't have a target rig but have townRoot, use gastown
			if targetRig == "" {
				targetRig = "gastown"
				rigPath = filepath.Join(townRoot, "gastown")
			}
		} else {
			// No town root found, fall back to gastown without rigPath
			targetRig = "gastown"
		}
	} else {
		// If rig specified, construct path
		townRoot, err := workspace.FindFromCwd()
		if err == nil && townRoot != "" {
			rigPath = filepath.Join(townRoot, targetRig)
		}
	}

	// Get formula name from args or default
	var formulaName string
	if len(args) > 0 {
		formulaName = args[0]
	} else {
		// Try to get default formula from rig config
		if rigPath != "" {
			formulaName = config.GetDefaultFormula(rigPath)
		}
		if formulaName == "" {
			return fmt.Errorf("no formula specified and no default formula configured\n\nTo set a default formula, add to your rig's settings/config.json:\n  \"workflow\": {\n    \"default_formula\": \"<formula-name>\"\n  }")
		}
		fmt.Printf("%s Using default formula: %s\n", style.Dim.Render("Note:"), formulaName)
	}

	// Find the formula file
	formulaPath, err := findFormulaFile(formulaName)
	if err != nil {
		return fmt.Errorf("finding formula: %w", err)
	}

	// Parse the formula
	f, err := parseFormulaFile(formulaPath)
	if err != nil {
		return fmt.Errorf("parsing formula: %w", err)
	}

	// Handle dry-run mode
	if formulaRunDryRun {
		return dryRunFormula(f, formulaName, targetRig)
	}

	// Currently only convoy formulas are supported for execution
	if f.Type != "convoy" {
		fmt.Printf("%s Formula type '%s' not yet supported for execution.\n",
			style.Dim.Render("Note:"), f.Type)
		fmt.Printf("Currently only 'convoy' formulas can be run.\n")
		fmt.Printf("\nTo run '%s' manually:\n", formulaName)
		fmt.Printf("  1. View formula:   gt formula show %s\n", formulaName)
		fmt.Printf("  2. Cook to proto:  bd cook %s\n", formulaName)
		fmt.Printf("  3. Pour molecule:  bd pour %s\n", formulaName)
		fmt.Printf("  4. Sling to rig:   gt sling <mol-id> %s\n", targetRig)
		return nil
	}

	// Execute convoy formula
	return executeConvoyFormula(f, formulaName, targetRig)
}

// dryRunFormula shows what would happen without executing
func dryRunFormula(f *formulaData, formulaName, targetRig string) error {
	fmt.Printf("%s Would execute formula:\n", style.Dim.Render("[dry-run]"))
	fmt.Printf("  Formula: %s\n", style.Bold.Render(formulaName))
	fmt.Printf("  Type:    %s\n", f.Type)
	fmt.Printf("  Rig:     %s\n", targetRig)
	if formulaRunPR > 0 {
		fmt.Printf("  PR:      #%d\n", formulaRunPR)
	}

	if f.Type == "convoy" && len(f.Legs) > 0 {
		// Generate review ID for dry-run display
		reviewID := generateFormulaShortID()

		// Build target description
		var targetDescription string
		if formulaRunPR > 0 {
			targetDescription = fmt.Sprintf("PR #%d", formulaRunPR)
		} else {
			targetDescription = "local files"
		}

		// Fetch PR info if --pr flag is set
		var prTitle string
		var changedFiles []map[string]interface{}
		if formulaRunPR > 0 {
			prTitle, changedFiles = fetchPRInfo(formulaRunPR)
			if prTitle != "" {
				fmt.Printf("  PR Title: %s\n", prTitle)
			}
			if len(changedFiles) > 0 {
				fmt.Printf("  Changed files: %d\n", len(changedFiles))
			}
		}

		// Show output directory if configured
		var outputDir string
		if f.Output != nil && f.Output.Directory != "" {
			dirCtx := map[string]interface{}{
				"review_id":    reviewID,
				"formula_name": formulaName,
			}
			outputDir = renderTemplateOrDefault(f.Output.Directory, dirCtx, ".reviews/"+reviewID)
			fmt.Printf("\n  Output directory: %s\n", outputDir)
		}

		fmt.Printf("\n  Legs (%d parallel):\n", len(f.Legs))
		for _, leg := range f.Legs {
			// Show rendered output path for each leg
			if f.Output != nil && outputDir != "" {
				legCtx := map[string]interface{}{
					"formula_name":       formulaName,
					"target_description": targetDescription,
					"review_id":          reviewID,
					"pr_number":          formulaRunPR,
					"pr_title":           prTitle,
					"leg": map[string]interface{}{
						"id":          leg.ID,
						"title":       leg.Title,
						"focus":       leg.Focus,
						"description": leg.Description,
					},
					"changed_files": changedFiles,
				}
				legPattern := renderTemplateOrDefault(f.Output.LegPattern, legCtx, leg.ID+"-findings.md")
				outputPath := filepath.Join(outputDir, legPattern)
				fmt.Printf("    • %s: %s\n      → %s\n", leg.ID, leg.Title, outputPath)
			} else {
				fmt.Printf("    • %s: %s\n", leg.ID, leg.Title)
			}
		}
		if f.Synthesis != nil {
			fmt.Printf("\n  Synthesis:\n")
			if f.Output != nil && outputDir != "" {
				synthPath := filepath.Join(outputDir, f.Output.Synthesis)
				fmt.Printf("    • %s\n      → %s\n", f.Synthesis.Title, synthPath)
			} else {
				fmt.Printf("    • %s\n", f.Synthesis.Title)
			}
		}
	}

	return nil
}

// executeConvoyFormula spawns a convoy of polecats to execute a convoy formula
func executeConvoyFormula(f *formulaData, formulaName, targetRig string) error {
	fmt.Printf("%s Executing convoy formula: %s\n\n",
		style.Bold.Render("🚚"), formulaName)

	// Get town beads directory for convoy creation
	townRoot, err := workspace.FindFromCwd()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}
	townBeads := filepath.Join(townRoot, ".beads")

	// Step 1: Create convoy bead
	convoyID := fmt.Sprintf("hq-cv-%s", generateFormulaShortID())
	convoyTitle := fmt.Sprintf("%s: %s", formulaName, f.Description)
	if len(convoyTitle) > 80 {
		convoyTitle = convoyTitle[:77] + "..."
	}

	// Build description with formula context
	description := fmt.Sprintf("Formula convoy: %s\n\nLegs: %d\nRig: %s",
		formulaName, len(f.Legs), targetRig)
	if formulaRunPR > 0 {
		description += fmt.Sprintf("\nPR: #%d", formulaRunPR)
	}

	createArgs := []string{
		"create",
		"--type=convoy",
		"--id=" + convoyID,
		"--title=" + convoyTitle,
		"--description=" + description,
	}
	if beads.NeedsForceForID(convoyID) {
		createArgs = append(createArgs, "--force")
	}

	createCmd := exec.Command("bd", createArgs...)
	createCmd.Dir = townBeads
	createCmd.Stderr = os.Stderr
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("creating convoy bead: %w", err)
	}

	fmt.Printf("%s Created convoy: %s\n", style.Bold.Render("✓"), convoyID)

	// Generate a unique review ID for this convoy run
	reviewID := generateFormulaShortID()

	// Build target description
	var targetDescription string
	if formulaRunPR > 0 {
		targetDescription = fmt.Sprintf("PR #%d", formulaRunPR)
	} else {
		targetDescription = "local files"
	}

	// Fetch PR info if --pr flag is set
	var prTitle string
	var changedFiles []map[string]interface{}
	if formulaRunPR > 0 {
		prTitle, changedFiles = fetchPRInfo(formulaRunPR)
	}

	// Create output directory if configured
	var outputDir string
	if f.Output != nil && f.Output.Directory != "" {
		// Build minimal context for directory rendering
		dirCtx := map[string]interface{}{
			"review_id":    reviewID,
			"formula_name": formulaName,
		}
		outputDir = renderTemplateOrDefault(f.Output.Directory, dirCtx, ".reviews/"+reviewID)

		// Create the directory
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			fmt.Printf("%s Failed to create output directory %s: %v\n",
				style.Dim.Render("Warning:"), outputDir, err)
		} else {
			fmt.Printf("  %s Output directory: %s\n", style.Dim.Render("📁"), outputDir)
		}
	}

	// Step 2: Create leg beads and track them
	legBeads := make(map[string]string) // leg.ID -> bead ID
	for _, leg := range f.Legs {
		legBeadID := fmt.Sprintf("hq-leg-%s", generateFormulaShortID())

		// Build leg description with prompt if available
		legDesc := leg.Description
		if f.Prompts != nil {
			if basePrompt, ok := f.Prompts["base"]; ok {
				// Build template context for this leg
				legCtx := map[string]interface{}{
					"formula_name":       formulaName,
					"target_description": targetDescription,
					"review_id":          reviewID,
					"pr_number":          formulaRunPR,
					"pr_title":           prTitle,
					"leg": map[string]interface{}{
						"id":          leg.ID,
						"title":       leg.Title,
						"focus":       leg.Focus,
						"description": leg.Description,
					},
					"changed_files": changedFiles,
					"files":         []string{}, // TODO: support --files flag
				}

				// Compute output path for this leg
				if f.Output != nil {
					legPattern := renderTemplateOrDefault(f.Output.LegPattern, legCtx, leg.ID+"-findings.md")
					outputPath := filepath.Join(outputDir, legPattern)
					legCtx["output_path"] = outputPath
					legCtx["output"] = map[string]interface{}{
						"directory": outputDir,
						"synthesis": f.Output.Synthesis,
					}
				}

				// Render the base prompt with template context
				renderedPrompt, err := renderTemplate(basePrompt, legCtx)
				if err != nil {
					fmt.Printf("%s Failed to render template for %s: %v\n",
						style.Dim.Render("Warning:"), leg.ID, err)
					renderedPrompt = basePrompt // Fall back to raw template
				}
				legDesc = fmt.Sprintf("%s\n\n---\nBase Prompt:\n%s", leg.Description, renderedPrompt)
			}
		}

		legArgs := []string{
			"create",
			"--type=task",
			"--id=" + legBeadID,
			"--title=" + leg.Title,
			"--description=" + legDesc,
		}
		if beads.NeedsForceForID(legBeadID) {
			legArgs = append(legArgs, "--force")
		}

		legCmd := exec.Command("bd", legArgs...)
		legCmd.Dir = townBeads
		legCmd.Stderr = os.Stderr
		if err := legCmd.Run(); err != nil {
			fmt.Printf("%s Failed to create leg bead for %s: %v\n",
				style.Dim.Render("Warning:"), leg.ID, err)
			continue
		}

		// Track the leg with the convoy
		trackArgs := []string{"dep", "add", convoyID, legBeadID, "--type=tracks"}
		trackCmd := exec.Command("bd", trackArgs...)
		trackCmd.Dir = townBeads
		if err := trackCmd.Run(); err != nil {
			fmt.Printf("%s Failed to track leg %s: %v\n",
				style.Dim.Render("Warning:"), leg.ID, err)
		}

		legBeads[leg.ID] = legBeadID
		fmt.Printf("  %s Created leg: %s (%s)\n", style.Dim.Render("○"), leg.ID, legBeadID)
	}

	// Step 3: Create synthesis bead if defined
	var synthesisBeadID string
	if f.Synthesis != nil {
		synthesisBeadID = fmt.Sprintf("hq-syn-%s", generateFormulaShortID())

		synDesc := f.Synthesis.Description
		if synDesc == "" {
			synDesc = "Synthesize findings from all legs into unified output"
		}

		synArgs := []string{
			"create",
			"--type=task",
			"--id=" + synthesisBeadID,
			"--title=" + f.Synthesis.Title,
			"--description=" + synDesc,
		}
		if beads.NeedsForceForID(synthesisBeadID) {
			synArgs = append(synArgs, "--force")
		}

		synCmd := exec.Command("bd", synArgs...)
		synCmd.Dir = townBeads
		synCmd.Stderr = os.Stderr
		if err := synCmd.Run(); err != nil {
			fmt.Printf("%s Failed to create synthesis bead: %v\n",
				style.Dim.Render("Warning:"), err)
		} else {
			// Track synthesis with convoy
			trackArgs := []string{"dep", "add", convoyID, synthesisBeadID, "--type=tracks"}
			trackCmd := exec.Command("bd", trackArgs...)
			trackCmd.Dir = townBeads
			_ = trackCmd.Run()

			// Add dependencies: synthesis depends on all legs
			for _, legBeadID := range legBeads {
				depArgs := []string{"dep", "add", synthesisBeadID, legBeadID}
				depCmd := exec.Command("bd", depArgs...)
				depCmd.Dir = townBeads
				_ = depCmd.Run()
			}

			fmt.Printf("  %s Created synthesis: %s\n", style.Dim.Render("★"), synthesisBeadID)
		}
	}

	// Step 4: Sling each leg to a polecat
	fmt.Printf("\n%s Dispatching legs to polecats...\n\n", style.Bold.Render("→"))

	slingCount := 0
	for _, leg := range f.Legs {
		legBeadID, ok := legBeads[leg.ID]
		if !ok {
			continue
		}

		// Build context message for the polecat
		contextMsg := fmt.Sprintf("Convoy leg: %s\nFocus: %s", leg.Title, leg.Focus)

		// Use gt sling with args for leg-specific context
		slingArgs := []string{
			"sling", legBeadID, targetRig,
			"-a", leg.Description,
			"-s", leg.Title,
		}

		slingCmd := exec.Command("gt", slingArgs...)
		slingCmd.Stdout = os.Stdout
		slingCmd.Stderr = os.Stderr

		if err := slingCmd.Run(); err != nil {
			fmt.Printf("%s Failed to sling leg %s: %v\n",
				style.Dim.Render("Warning:"), leg.ID, err)
			// Add comment to bead about failure
			commentArgs := []string{"comment", legBeadID, fmt.Sprintf("Failed to sling: %v", err)}
			commentCmd := exec.Command("bd", commentArgs...)
			commentCmd.Dir = townBeads
			_ = commentCmd.Run()
			continue
		}

		slingCount++
		_ = contextMsg // Used in future for richer context
	}

	// Summary
	fmt.Printf("\n%s Convoy dispatched!\n", style.Bold.Render("✓"))
	fmt.Printf("  Convoy:  %s\n", convoyID)
	fmt.Printf("  Legs:    %d dispatched\n", slingCount)
	if synthesisBeadID != "" {
		fmt.Printf("  Synthesis: %s (blocked until legs complete)\n", synthesisBeadID)
	}
	fmt.Printf("\n  Track progress: gt convoy status %s\n", convoyID)

	return nil
}

// formulaData holds parsed formula information
type formulaData struct {
	Name        string
	Description string
	Type        string
	Legs        []formulaLeg
	Synthesis   *formulaSynthesis
	Prompts     map[string]string
	Output      *formulaOutput
}

type formulaOutput struct {
	Directory  string
	LegPattern string
	Synthesis  string
}

type formulaLeg struct {
	ID          string
	Title       string
	Focus       string
	Description string
}

type formulaSynthesis struct {
	Title       string
	Description string
	DependsOn   []string
}

// FormulaSource indicates where a formula was found
type FormulaSource int

const (
	// FormulaSourceFile indicates the formula was found on disk
	FormulaSourceFile FormulaSource = iota
	// FormulaSourceEmbedded indicates the formula was found in embedded resources
	FormulaSourceEmbedded
)

// FormulaLocation contains the result of formula resolution
type FormulaLocation struct {
	// Path is the file path (for FormulaSourceFile) or the formula name (for FormulaSourceEmbedded)
	Path string
	// Source indicates where the formula was found
	Source FormulaSource
}

// IsEmbedded returns true if the formula is from embedded resources
func (f FormulaLocation) IsEmbedded() bool {
	return f.Source == FormulaSourceEmbedded
}

// findFormulaFile searches for a formula file by name
// Resolution order: rig .beads/formulas/ → town $GT_ROOT/.beads/formulas/ → embedded
func findFormulaFile(name string) (string, error) {
	loc, err := findFormulaWithSource(name)
	if err != nil {
		return "", err
	}
	// For backwards compatibility, return the path for file sources
	// or "embedded:<name>" marker for embedded sources
	if loc.IsEmbedded() {
		return "embedded:" + name, nil
	}
	return loc.Path, nil
}

// findFormulaWithSource searches for a formula by name and returns its location
// Resolution order: rig .beads/formulas/ → town $GT_ROOT/.beads/formulas/ → embedded
func findFormulaWithSource(name string) (FormulaLocation, error) {
	// Search paths in order
	searchPaths := []string{}

	// 1. Rig .beads/formulas/ (project-level)
	if cwd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(cwd, ".beads", "formulas"))
	}

	// 2. Town $GT_ROOT/.beads/formulas/
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(townRoot, ".beads", "formulas"))
	}

	// Try each path with common extensions
	extensions := []string{".formula.toml", ".formula.json"}
	for _, basePath := range searchPaths {
		for _, ext := range extensions {
			path := filepath.Join(basePath, name+ext)
			if _, err := os.Stat(path); err == nil {
				return FormulaLocation{Path: path, Source: FormulaSourceFile}, nil
			}
		}
	}

	// 3. Embedded formulas (final fallback)
	if formula.EmbeddedFormulaExists(name) {
		return FormulaLocation{Path: name, Source: FormulaSourceEmbedded}, nil
	}

	return FormulaLocation{}, fmt.Errorf("formula '%s' not found in search paths or embedded", name)
}

// parseFormulaFile parses a formula file into formulaData
// Handles both file paths and "embedded:<name>" markers
func parseFormulaFile(path string) (*formulaData, error) {
	var data []byte
	var err error

	// Check if this is an embedded formula marker
	if strings.HasPrefix(path, "embedded:") {
		name := strings.TrimPrefix(path, "embedded:")
		data, err = formula.GetEmbeddedFormula(name)
		if err != nil {
			return nil, fmt.Errorf("reading embedded formula: %w", err)
		}
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	// Use simple TOML parsing for the fields we need
	// (avoids importing the full formula package which might cause cycles)
	f := &formulaData{
		Prompts: make(map[string]string),
	}

	content := string(data)

	// Parse formula name
	if match := extractTOMLValue(content, "formula"); match != "" {
		f.Name = match
	}

	// Parse description
	if match := extractTOMLMultiline(content, "description"); match != "" {
		f.Description = match
	}

	// Parse type
	if match := extractTOMLValue(content, "type"); match != "" {
		f.Type = match
	}

	// Parse legs (convoy formulas)
	f.Legs = extractLegs(content)

	// Parse synthesis
	f.Synthesis = extractSynthesis(content)

	// Parse prompts
	f.Prompts = extractPrompts(content)

	// Parse output config
	f.Output = extractOutput(content)

	return f, nil
}

// extractTOMLValue extracts a simple quoted value from TOML
func extractTOMLValue(content, key string) string {
	// Match: key = "value" or key = 'value'
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+" =") || strings.HasPrefix(line, key+"=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				// Remove quotes
				if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') {
					return val[1 : len(val)-1]
				}
				return val
			}
		}
	}
	return ""
}

// extractTOMLMultiline extracts a multiline string (""" ... """)
func extractTOMLMultiline(content, key string) string {
	// Look for key = """
	keyPattern := key + ` = """`
	idx := strings.Index(content, keyPattern)
	if idx == -1 {
		// Try single-line
		return extractTOMLValue(content, key)
	}

	start := idx + len(keyPattern)
	end := strings.Index(content[start:], `"""`)
	if end == -1 {
		return ""
	}

	return strings.TrimSpace(content[start : start+end])
}

// extractLegs parses [[legs]] sections from TOML
func extractLegs(content string) []formulaLeg {
	var legs []formulaLeg

	// Split by [[legs]]
	sections := strings.Split(content, "[[legs]]")
	for i, section := range sections {
		if i == 0 {
			continue // Skip content before first [[legs]]
		}

		// Find where this section ends (next [[ or EOF)
		endIdx := strings.Index(section, "[[")
		if endIdx == -1 {
			endIdx = len(section)
		}
		section = section[:endIdx]

		leg := formulaLeg{
			ID:          extractTOMLValue(section, "id"),
			Title:       extractTOMLValue(section, "title"),
			Focus:       extractTOMLValue(section, "focus"),
			Description: extractTOMLMultiline(section, "description"),
		}

		if leg.ID != "" {
			legs = append(legs, leg)
		}
	}

	return legs
}

// extractSynthesis parses [synthesis] section from TOML
func extractSynthesis(content string) *formulaSynthesis {
	idx := strings.Index(content, "[synthesis]")
	if idx == -1 {
		return nil
	}

	section := content[idx:]
	// Find where section ends
	if endIdx := strings.Index(section[1:], "\n["); endIdx != -1 {
		section = section[:endIdx+1]
	}

	syn := &formulaSynthesis{
		Title:       extractTOMLValue(section, "title"),
		Description: extractTOMLMultiline(section, "description"),
	}

	// Parse depends_on array
	if depsLine := extractTOMLValue(section, "depends_on"); depsLine != "" {
		// Simple array parsing: ["a", "b", "c"]
		depsLine = strings.Trim(depsLine, "[]")
		for _, dep := range strings.Split(depsLine, ",") {
			dep = strings.Trim(strings.TrimSpace(dep), `"'`)
			if dep != "" {
				syn.DependsOn = append(syn.DependsOn, dep)
			}
		}
	}

	if syn.Title == "" && syn.Description == "" {
		return nil
	}

	return syn
}

// extractPrompts parses [prompts] section from TOML
func extractPrompts(content string) map[string]string {
	prompts := make(map[string]string)

	idx := strings.Index(content, "[prompts]")
	if idx == -1 {
		return prompts
	}

	section := content[idx:]
	// Find where section ends
	if endIdx := strings.Index(section[1:], "\n["); endIdx != -1 {
		section = section[:endIdx+1]
	}

	// Extract base prompt
	if base := extractTOMLMultiline(section, "base"); base != "" {
		prompts["base"] = base
	}

	return prompts
}

// extractOutput parses [output] section from TOML
func extractOutput(content string) *formulaOutput {
	idx := strings.Index(content, "[output]")
	if idx == -1 {
		return nil
	}

	section := content[idx:]
	// Find where section ends (next [ that isn't part of output)
	if endIdx := strings.Index(section[1:], "\n["); endIdx != -1 {
		section = section[:endIdx+1]
	}

	out := &formulaOutput{
		Directory:  extractTOMLValue(section, "directory"),
		LegPattern: extractTOMLValue(section, "leg_pattern"),
		Synthesis:  extractTOMLValue(section, "synthesis"),
	}

	if out.Directory == "" && out.LegPattern == "" && out.Synthesis == "" {
		return nil
	}

	return out
}

// renderTemplate renders a Go text/template with the given context map
func renderTemplate(tmplText string, ctx map[string]interface{}) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplText)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}
	return buf.String(), nil
}

// renderTemplateOrDefault renders a template, returning defaultVal on error
func renderTemplateOrDefault(tmplText string, ctx map[string]interface{}, defaultVal string) string {
	if tmplText == "" {
		return defaultVal
	}
	result, err := renderTemplate(tmplText, ctx)
	if err != nil {
		return defaultVal
	}
	return result
}

// fetchPRInfo fetches PR title and changed files from GitHub using gh CLI
func fetchPRInfo(prNumber int) (string, []map[string]interface{}) {
	var prTitle string
	var changedFiles []map[string]interface{}

	// Get PR title
	titleCmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "title", "--jq", ".title")
	titleOut, err := titleCmd.Output()
	if err == nil {
		prTitle = strings.TrimSpace(string(titleOut))
	}

	// Get changed files with stats
	filesCmd := exec.Command("gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "files", "--jq", ".files[] | \"\\(.path) \\(.additions) \\(.deletions)\"")
	filesOut, err := filesCmd.Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(filesOut)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				additions, err := strconv.Atoi(parts[1])
				if err != nil {
					continue
				}
				deletions, err := strconv.Atoi(parts[2])
				if err != nil {
					continue
				}
				changedFiles = append(changedFiles, map[string]interface{}{
					"path":      parts[0],
					"additions": additions,
					"deletions": deletions,
				})
			}
		}
	}

	return prTitle, changedFiles
}

// generateFormulaShortID generates a short random ID (5 lowercase chars)
func generateFormulaShortID() string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return strings.ToLower(base32.StdEncoding.EncodeToString(b)[:5])
}

// runFormulaCreate creates a new formula template
func runFormulaCreate(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	// Find or create formulas directory
	formulasDir := ".beads/formulas"

	// Check if we're in a beads-enabled directory
	if _, err := os.Stat(".beads"); os.IsNotExist(err) {
		// Try user formulas directory
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot find home directory: %w", err)
		}
		formulasDir = filepath.Join(home, ".beads", "formulas")
	}

	// Ensure directory exists
	if err := os.MkdirAll(formulasDir, 0755); err != nil {
		return fmt.Errorf("creating formulas directory: %w", err)
	}

	// Generate filename
	filename := filepath.Join(formulasDir, formulaName+".formula.toml")

	// Check if file already exists
	if _, err := os.Stat(filename); err == nil {
		return fmt.Errorf("formula already exists: %s", filename)
	}

	// Generate template based on type
	var template string
	switch formulaCreateType {
	case "task":
		template = generateTaskTemplate(formulaName)
	case "workflow":
		template = generateWorkflowTemplate(formulaName)
	case "patrol":
		template = generatePatrolTemplate(formulaName)
	default:
		return fmt.Errorf("unknown formula type: %s (use: task, workflow, or patrol)", formulaCreateType)
	}

	// Write the file
	if err := os.WriteFile(filename, []byte(template), 0644); err != nil {
		return fmt.Errorf("writing formula file: %w", err)
	}

	fmt.Printf("%s Created formula: %s\n", style.Bold.Render("✓"), filename)
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Edit the formula: %s\n", filename)
	fmt.Printf("  2. View it:          gt formula show %s\n", formulaName)
	fmt.Printf("  3. Run it:           gt formula run %s\n", formulaName)

	return nil
}

func generateTaskTemplate(name string) string {
	// Sanitize name for use in template
	title := strings.ReplaceAll(name, "-", " ")
	title = cases.Title(language.English).String(title)

	return fmt.Sprintf(`# Formula: %s
# Type: task
# Created by: gt formula create

description = """%s task.

Add a detailed description here."""
formula = "%s"
version = 1

# Single step task
[[steps]]
id = "do-task"
title = "Execute task"
description = """
Perform the main task work.

**Steps:**
1. Understand the requirements
2. Implement the changes
3. Verify the work
"""

# Variables that can be passed when running the formula
# [vars]
# [vars.issue]
# description = "Issue ID to work on"
# required = true
#
# [vars.target]
# description = "Target branch"
# default = "main"
`, name, title, name)
}

func generateWorkflowTemplate(name string) string {
	title := strings.ReplaceAll(name, "-", " ")
	title = cases.Title(language.English).String(title)

	return fmt.Sprintf(`# Formula: %s
# Type: workflow
# Created by: gt formula create

description = """%s workflow.

A multi-step workflow with dependencies between steps."""
formula = "%s"
version = 1

# Step 1: Setup
[[steps]]
id = "setup"
title = "Setup environment"
description = """
Prepare the environment for the workflow.

**Steps:**
1. Check prerequisites
2. Set up working environment
"""

# Step 2: Implementation (depends on setup)
[[steps]]
id = "implement"
title = "Implement changes"
needs = ["setup"]
description = """
Make the necessary code changes.

**Steps:**
1. Understand requirements
2. Write code
3. Test locally
"""

# Step 3: Test (depends on implementation)
[[steps]]
id = "test"
title = "Run tests"
needs = ["implement"]
description = """
Verify the changes work correctly.

**Steps:**
1. Run unit tests
2. Run integration tests
3. Check for regressions
"""

# Step 4: Complete (depends on tests)
[[steps]]
id = "complete"
title = "Complete workflow"
needs = ["test"]
description = """
Finalize and clean up.

**Steps:**
1. Commit final changes
2. Clean up temporary files
"""

# Variables
[vars]
[vars.issue]
description = "Issue ID to work on"
required = true
`, name, title, name)
}

func generatePatrolTemplate(name string) string {
	title := strings.ReplaceAll(name, "-", " ")
	title = cases.Title(language.English).String(title)

	return fmt.Sprintf(`# Formula: %s
# Type: patrol
# Created by: gt formula create
#
# Patrol formulas are for repeating cycles (wisps).
# They run continuously and are NOT synced to git.

description = """%s patrol.

A patrol formula for periodic checks. Patrol formulas create wisps
(ephemeral molecules) that are NOT synced to git."""
formula = "%s"
version = 1

# The patrol step(s)
[[steps]]
id = "check"
title = "Run patrol check"
description = """
Perform the patrol inspection.

**Check for:**
1. Health indicators
2. Warning signs
3. Items needing attention

**On findings:**
- Log the issue
- Escalate if critical
"""

# Optional: remediation step
# [[steps]]
# id = "remediate"
# title = "Fix issues"
# needs = ["check"]
# description = """
# Fix any issues found during the check.
# """

# Variables (optional)
# [vars]
# [vars.verbose]
# description = "Enable verbose output"
# default = "false"
`, name, title, name)
}

// promptYesNo asks the user a yes/no question
func promptYesNo(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

// runFormulaModify copies an embedded formula for customization
func runFormulaModify(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	// Check if formula exists in embedded
	if !formula.EmbeddedFormulaExists(formulaName) {
		return fmt.Errorf("formula '%s' not found in embedded formulas.\n\nUse 'gt formula list' to see available formulas.", formulaName)
	}

	// Determine destination path
	var destDir string
	var destDescription string

	if formulaModifyRig != "" {
		// Copy to rig level
		townRoot, err := findTownRoot()
		if err != nil {
			return fmt.Errorf("finding town root: %w", err)
		}
		destDir = filepath.Join(townRoot, formulaModifyRig, ".beads", "formulas")
		destDescription = fmt.Sprintf("rig '%s'", formulaModifyRig)
	} else if formulaModifyTown != "" {
		// Explicit town path override
		destDir = filepath.Join(formulaModifyTown, ".beads", "formulas")
		destDescription = "specified town path"
	} else {
		// Default: copy to town level
		townRoot, err := findTownRoot()
		if err != nil {
			return fmt.Errorf("finding town root: %w", err)
		}
		destDir = filepath.Join(townRoot, ".beads", "formulas")
		destDescription = "town level"
	}

	// Check if override already exists
	filename := formulaName + ".formula.toml"
	destPath := filepath.Join(destDir, filename)
	if _, err := os.Stat(destPath); err == nil {
		return fmt.Errorf("Override already exists at %s. Use 'gt formula reset %s' to remove it first.", destPath, formulaName)
	}

	// Copy the formula
	copiedPath, err := formula.CopyFormulaTo(formulaName, destDir)
	if err != nil {
		return fmt.Errorf("copying formula: %w", err)
	}

	// Print success message and modification guide
	fmt.Printf("Formula copied to: %s\n\n", copiedPath)
	printFormulaModificationGuide()

	_ = destDescription // for future use in more detailed messages

	return nil
}

// printFormulaModificationGuide prints the formula modification guide
func printFormulaModificationGuide() {
	guide := `== Formula Modification Guide ==

Formula Structure:
  formula = "name"           # Formula identifier
  type = "workflow"          # workflow | convoy | aspect | expansion
  version = 1                # Increment when making breaking changes
  description = "..."        # What this formula does

Steps (for workflow type):
  [[steps]]
  id = "step-id"             # Unique identifier
  title = "Step Title"       # Human-readable name
  needs = ["other-step"]     # Dependencies (optional)
  description = """          # Instructions for the agent
  What to do in this step...
  """

Variables:
  [vars.myvar]
  description = "What this variable is for"
  required = true            # or false with default
  default = "value"          # Default if not required

Resolution Order:
  1. Rig:   <rig>/.beads/formulas/     (most specific)
  2. Town:  $GT_ROOT/.beads/formulas/  (user customizations)
  3. Embedded: (compiled in binary)     (defaults)

Commands:
  gt formula diff <name>     # See your changes vs embedded
  gt formula reset <name>    # Remove override, restore embedded
  gt formula show <name>     # View formula details
`
	fmt.Print(guide)
}

// FormulaOverride represents a formula override at a specific location
type FormulaOverride struct {
	Name       string
	Path       string
	Level      string // "rig" or "town"
	RigName    string // Only set if Level == "rig"
	IsEmbedded bool
	LinesDiff  int // Approximate line count difference from embedded
}

// runFormulaDiff shows formula overrides - summary or detailed view
func runFormulaDiff(cmd *cobra.Command, args []string) error {
	if len(args) == 1 {
		return runFormulaDiffDetailed(args[0])
	}
	return runFormulaDiffSummary()
}

// runFormulaDiffSummary shows a visual map of all formula overrides
func runFormulaDiffSummary() error {
	townRoot, err := findTownRoot()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Get all embedded formula names
	embeddedNames, err := formula.GetEmbeddedFormulaNames()
	if err != nil {
		return fmt.Errorf("reading embedded formulas: %w", err)
	}

	// Scan for overrides
	overrides := scanAllFormulaOverrides(townRoot)

	// Also find custom formulas (not in embedded)
	customFormulas := findCustomFormulas(townRoot, embeddedNames)

	// Check if we have any overrides or custom formulas
	if len(overrides) == 0 && len(customFormulas) == 0 {
		fmt.Printf("No formula overrides found.\n")
		fmt.Printf("All formulas using embedded defaults (%d formulas available).\n\n", len(embeddedNames))
		fmt.Printf("Run 'gt formula modify <name>' to customize a formula.\n")
		return nil
	}

	// Print header
	fmt.Printf("Formula Override Map\n")
	fmt.Printf("════════════════════\n\n")

	fmt.Printf("                            RESOLUTION ORDER\n")
	fmt.Printf("      ┌─────────────────────────────────────────────────────┐\n")
	fmt.Printf("      │  Rig Override  →  Town Override  →  Embedded        │\n")
	fmt.Printf("      └─────────────────────────────────────────────────────┘\n\n")

	// Group overrides by formula name
	overridesByName := make(map[string][]FormulaOverride)
	for _, o := range overrides {
		overridesByName[o.Name] = append(overridesByName[o.Name], o)
	}

	// Count stats
	usingEmbedded := 0
	withOverride := 0
	customCount := len(customFormulas)

	// Show embedded formulas that have overrides
	for _, name := range embeddedNames {
		ovrs, hasOverride := overridesByName[name]
		if !hasOverride {
			usingEmbedded++
			continue // Skip formulas using embedded (no override)
		}
		withOverride++

		fmt.Printf("%s\n", style.Bold.Render(name))

		// Determine what's active
		var townOverride, rigOverride *FormulaOverride
		for i := range ovrs {
			if ovrs[i].Level == "town" {
				townOverride = &ovrs[i]
			} else if ovrs[i].Level == "rig" {
				rigOverride = &ovrs[i]
			}
		}

		// Build the resolution diagram
		if rigOverride != nil && townOverride != nil {
			// Both town and rig overrides
			fmt.Printf("    embedded ─┬─► town override\n")
			fmt.Printf("              │   %s\n", style.Dim.Render(townOverride.Path))
			fmt.Printf("              │\n")
			fmt.Printf("              └─► rig override (%s) ──────────── %s\n", rigOverride.RigName, style.Bold.Render("✓ active"))
			fmt.Printf("                  %s\n", style.Dim.Render(rigOverride.Path))
		} else if rigOverride != nil {
			// Only rig override
			fmt.Printf("    embedded ───► rig override (%s) ──────────── %s\n", rigOverride.RigName, style.Bold.Render("✓ active"))
			fmt.Printf("                  %s\n", style.Dim.Render(rigOverride.Path))
		} else if townOverride != nil {
			// Only town override
			fmt.Printf("    embedded ───► town override ─────────────────── %s\n", style.Bold.Render("✓ active"))
			fmt.Printf("                  %s\n", style.Dim.Render(townOverride.Path))
		}

		fmt.Println()
	}

	// Show custom formulas (not in embedded)
	if len(customFormulas) > 0 {
		for _, cf := range customFormulas {
			fmt.Printf("%s\n", style.Bold.Render(cf.Name))
			fmt.Printf("    (not in embedded) ─► %s (%s) ──────────── %s\n", cf.Level, cf.RigName, style.Dim.Render("custom"))
			fmt.Printf("                         %s\n", style.Dim.Render(cf.Path))
			fmt.Println()
		}
	}

	// Print summary
	fmt.Printf("───────────────────────────────────────────────────────────\n")
	fmt.Printf("Summary: %d using embedded, %d with override, %d custom\n", usingEmbedded, withOverride, customCount)
	fmt.Printf("Run 'gt formula diff <name>' for detailed diff\n")

	return nil
}

// runFormulaDiffDetailed shows side-by-side diff for a specific formula
func runFormulaDiffDetailed(name string) error {
	townRoot, err := findTownRoot()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Get embedded content if exists
	var embeddedContent []byte
	hasEmbedded := formula.EmbeddedFormulaExists(name)
	if hasEmbedded {
		embeddedContent, err = formula.GetEmbeddedFormula(name)
		if err != nil {
			return fmt.Errorf("reading embedded formula: %w", err)
		}
	}

	// Find overrides
	overrides := scanFormulaOverridesForName(townRoot, name)

	if !hasEmbedded && len(overrides) == 0 {
		return fmt.Errorf("formula '%s' not found anywhere.\n\nUse 'gt formula list' to see available formulas.", name)
	}

	// Print header
	fmt.Printf("%s\n", style.Bold.Render(name))

	// Print resolution chain
	if hasEmbedded {
		fmt.Printf("    ├─ embedded: (compiled in gt)\n")
	}

	var townOverride, rigOverride *FormulaOverride
	for i := range overrides {
		if overrides[i].Level == "town" {
			townOverride = &overrides[i]
			fmt.Printf("    ├─ town:     %s\n", townOverride.Path)
		} else if overrides[i].Level == "rig" {
			rigOverride = &overrides[i]
		}
	}
	if rigOverride != nil {
		fmt.Printf("    └─ rig:      %s  %s\n", rigOverride.Path, style.Bold.Render("◄ active"))
	} else if townOverride != nil {
		// Reprint town as active
		fmt.Printf("    (town is active)\n")
	} else if hasEmbedded {
		fmt.Printf("    (embedded is active - no overrides)\n")
	}

	fmt.Println()

	// If no overrides, just show the embedded content summary
	if len(overrides) == 0 {
		fmt.Printf("No overrides found for this formula.\n")
		fmt.Printf("Use 'gt formula modify %s' to create an override.\n", name)
		return nil
	}

	// Check if override's base version differs from current embedded (stale hash detection)
	if hasEmbedded && len(overrides) > 0 {
		// Check the active override (rig takes precedence over town)
		activeOverride := overrides[0]
		for _, o := range overrides {
			if o.Level == "rig" {
				activeOverride = o
				break
			}
		}
		overrideContent, readErr := os.ReadFile(activeOverride.Path)
		if readErr == nil {
			baseHash := formula.ExtractBaseHash(overrideContent)
			if baseHash != "" {
				currentHash, hashErr := formula.GetEmbeddedFormulaHash(name)
				if hashErr == nil && baseHash != currentHash {
					fmt.Printf("%s Embedded version has been updated since you created this override.\n",
						style.Bold.Render("⚠ Update available:"))
					fmt.Printf("  Base:    sha256:%s\n", truncateHash(baseHash))
					fmt.Printf("  Current: sha256:%s\n", truncateHash(currentHash))
					fmt.Printf("  Run 'gt formula update %s' to merge changes.\n\n", name)
				}
			}
		}
	}

	// Show diffs
	if hasEmbedded && townOverride != nil {
		fmt.Printf("[Embedded → Town]\n")
		printSimpleDiff(embeddedContent, townOverride.Path)
		fmt.Println()
	}

	if townOverride != nil && rigOverride != nil {
		fmt.Printf("[Town → Rig (active)]\n")
		townContent, err := os.ReadFile(townOverride.Path)
		if err == nil {
			printSimpleDiffContent(townContent, rigOverride.Path, "town override", "rig override")
		}
	} else if hasEmbedded && rigOverride != nil && townOverride == nil {
		fmt.Printf("[Embedded → Rig (active)]\n")
		printSimpleDiff(embeddedContent, rigOverride.Path)
	} else if !hasEmbedded && len(overrides) > 0 {
		fmt.Printf("Custom formula (not in embedded).\n")
		// Show content summary
		o := overrides[0]
		content, err := os.ReadFile(o.Path)
		if err == nil {
			lines := strings.Count(string(content), "\n")
			fmt.Printf("  %d lines at %s\n", lines, o.Path)
		}
	}

	return nil
}

// printSimpleDiff shows a simple unified diff between embedded content and a file
func printSimpleDiff(embeddedContent []byte, filePath string) error {
	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	return printSimpleDiffBytes(embeddedContent, fileContent, "embedded", filepath.Base(filepath.Dir(filePath)))
}

// printSimpleDiffContent shows a simple side-by-side comparison
func printSimpleDiffContent(leftContent []byte, rightPath, leftLabel, rightLabel string) error {
	rightContent, err := os.ReadFile(rightPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}
	return printSimpleDiffBytes(leftContent, rightContent, leftLabel, rightLabel)
}

// printSimpleDiffBytes shows a simple side-by-side comparison of two byte slices
func printSimpleDiffBytes(leftContent, rightContent []byte, leftLabel, rightLabel string) error {

	leftLines := strings.Split(string(leftContent), "\n")
	rightLines := strings.Split(string(rightContent), "\n")

	// Find differences
	diffs := findLineDifferences(leftLines, rightLines)

	if len(diffs) == 0 {
		fmt.Printf("  (no differences)\n")
		return nil
	}

	// Print header
	fmt.Printf("────────────────────────────────────────────────────────────────────────────\n")
	fmt.Printf("%-38s │ %s\n", leftLabel, rightLabel)
	fmt.Printf("────────────────────────────────────────────────────────────────────────────\n")

	// Print differences (limit to first 20 for readability)
	shown := 0
	for _, d := range diffs {
		if shown >= 20 {
			fmt.Printf("  ... (%d more differences)\n", len(diffs)-shown)
			break
		}

		left := truncateLine(d.Left, 36)
		right := truncateLine(d.Right, 36)

		if d.Type == "changed" {
			fmt.Printf("%-38s │ %s\n", left, right)
		} else if d.Type == "removed" {
			fmt.Printf("%-38s │ %s\n", left, style.Dim.Render("(removed)"))
		} else if d.Type == "added" {
			fmt.Printf("%-38s │ %s\n", style.Dim.Render("(added)"), right)
		}
		shown++
	}

	fmt.Printf("────────────────────────────────────────────────────────────────────────────\n")

	return nil
}

// LineDiff represents a difference between two lines
type LineDiff struct {
	Type  string // "changed", "added", "removed"
	Left  string
	Right string
}

// findLineDifferences finds lines that differ between two files
func findLineDifferences(left, right []string) []LineDiff {
	var diffs []LineDiff

	// Simple line-by-line comparison (not a proper diff algorithm)
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}

	for i := 0; i < maxLen; i++ {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}

		// Skip empty lines and comments that match
		if strings.TrimSpace(l) == strings.TrimSpace(r) {
			continue
		}

		// Skip if both are empty or whitespace only
		if strings.TrimSpace(l) == "" && strings.TrimSpace(r) == "" {
			continue
		}

		if i >= len(left) {
			diffs = append(diffs, LineDiff{Type: "added", Right: r})
		} else if i >= len(right) {
			diffs = append(diffs, LineDiff{Type: "removed", Left: l})
		} else {
			diffs = append(diffs, LineDiff{Type: "changed", Left: l, Right: r})
		}
	}

	return diffs
}

// truncateLine truncates a line to fit in the given width
func truncateLine(line string, width int) string {
	line = strings.TrimSpace(line)
	if len(line) <= width {
		return line
	}
	return line[:width-3] + "..."
}

// scanAllFormulaOverrides scans all locations for formula overrides
func scanAllFormulaOverrides(townRoot string) []FormulaOverride {
	var overrides []FormulaOverride

	// Scan town-level formulas
	townFormulasDir := filepath.Join(townRoot, ".beads", "formulas")
	if entries, err := os.ReadDir(townFormulasDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formula.toml") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".formula.toml")
			if formula.EmbeddedFormulaExists(name) {
				overrides = append(overrides, FormulaOverride{
					Name:  name,
					Path:  filepath.Join(townFormulasDir, entry.Name()),
					Level: "town",
				})
			}
		}
	}

	// Scan rig-level formulas
	rigDirs := discoverRigDirs(townRoot)
	for _, rigDir := range rigDirs {
		rigFormulasDir := filepath.Join(rigDir, ".beads", "formulas")
		if entries, err := os.ReadDir(rigFormulasDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formula.toml") {
					continue
				}
				name := strings.TrimSuffix(entry.Name(), ".formula.toml")
				if formula.EmbeddedFormulaExists(name) {
					overrides = append(overrides, FormulaOverride{
						Name:    name,
						Path:    filepath.Join(rigFormulasDir, entry.Name()),
						Level:   "rig",
						RigName: filepath.Base(rigDir),
					})
				}
			}
		}
	}

	return overrides
}

// scanFormulaOverridesForName scans for overrides of a specific formula
func scanFormulaOverridesForName(townRoot, name string) []FormulaOverride {
	var overrides []FormulaOverride
	filename := name + ".formula.toml"

	// Check town-level
	townPath := filepath.Join(townRoot, ".beads", "formulas", filename)
	if _, err := os.Stat(townPath); err == nil {
		overrides = append(overrides, FormulaOverride{
			Name:  name,
			Path:  townPath,
			Level: "town",
		})
	}

	// Check rig-level
	rigDirs := discoverRigDirs(townRoot)
	for _, rigDir := range rigDirs {
		rigPath := filepath.Join(rigDir, ".beads", "formulas", filename)
		if _, err := os.Stat(rigPath); err == nil {
			overrides = append(overrides, FormulaOverride{
				Name:    name,
				Path:    rigPath,
				Level:   "rig",
				RigName: filepath.Base(rigDir),
			})
		}
	}

	return overrides
}

// findCustomFormulas finds formulas that exist locally but not in embedded
func findCustomFormulas(townRoot string, embeddedNames []string) []FormulaOverride {
	var custom []FormulaOverride
	embeddedSet := make(map[string]bool)
	for _, n := range embeddedNames {
		embeddedSet[n] = true
	}

	// Check town-level
	townFormulasDir := filepath.Join(townRoot, ".beads", "formulas")
	if entries, err := os.ReadDir(townFormulasDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formula.toml") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".formula.toml")
			if !embeddedSet[name] {
				custom = append(custom, FormulaOverride{
					Name:  name,
					Path:  filepath.Join(townFormulasDir, entry.Name()),
					Level: "town",
				})
			}
		}
	}

	// Check rig-level
	rigDirs := discoverRigDirs(townRoot)
	for _, rigDir := range rigDirs {
		rigFormulasDir := filepath.Join(rigDir, ".beads", "formulas")
		if entries, err := os.ReadDir(rigFormulasDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".formula.toml") {
					continue
				}
				name := strings.TrimSuffix(entry.Name(), ".formula.toml")
				if !embeddedSet[name] {
					custom = append(custom, FormulaOverride{
						Name:    name,
						Path:    filepath.Join(rigFormulasDir, entry.Name()),
						Level:   "rig",
						RigName: filepath.Base(rigDir),
					})
				}
			}
		}
	}

	return custom
}

// discoverRigDirs returns paths to all rig directories in the town
func discoverRigDirs(townRoot string) []string {
	var rigDirs []string

	// Read rigs.json to get registered rigs
	rigsConfigPath := filepath.Join(townRoot, "mayor", "rigs.json")
	content, err := os.ReadFile(rigsConfigPath)
	if err != nil {
		return rigDirs
	}

	// Simple JSON parsing for rig names
	// Looking for "rigs": { "rigname": { ... } }
	lines := strings.Split(string(content), "\n")
	inRigs := false
	braceDepth := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, `"rigs"`) {
			inRigs = true
			continue
		}
		if inRigs {
			if strings.Contains(trimmed, "{") {
				braceDepth++
			}
			if strings.Contains(trimmed, "}") {
				braceDepth--
				if braceDepth <= 0 {
					inRigs = false
				}
			}
			// Look for rig name patterns like "rigname": {
			if braceDepth == 1 && strings.Contains(trimmed, `":`) {
				parts := strings.Split(trimmed, `"`)
				if len(parts) >= 2 {
					rigName := parts[1]
					if rigName != "" && rigName != "rigs" {
						rigPath := filepath.Join(townRoot, rigName)
						if info, err := os.Stat(rigPath); err == nil && info.IsDir() {
							rigDirs = append(rigDirs, rigPath)
						}
					}
				}
			}
		}
	}

	return rigDirs
}

// runFormulaReset removes a formula override
func runFormulaReset(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	townRoot, err := findTownRoot()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	filename := formulaName + ".formula.toml"

	// Determine which override to remove
	var targetPath string
	var targetLevel string

	if formulaResetRig != "" {
		// Remove from specific rig
		targetPath = filepath.Join(townRoot, formulaResetRig, ".beads", "formulas", filename)
		targetLevel = fmt.Sprintf("rig '%s'", formulaResetRig)
	} else {
		// Default: remove from town level
		targetPath = filepath.Join(townRoot, ".beads", "formulas", filename)
		targetLevel = "town"

		// But check if rig override also exists and warn
		rigDirs := discoverRigDirs(townRoot)
		for _, rigDir := range rigDirs {
			rigPath := filepath.Join(rigDir, ".beads", "formulas", filename)
			if _, err := os.Stat(rigPath); err == nil {
				// Both exist - require explicit flag
				if _, err := os.Stat(targetPath); err == nil {
					return fmt.Errorf("Both town and rig (%s) overrides exist for '%s'.\n\nUse --rig=%s to remove the rig override, or remove the town override first.",
						filepath.Base(rigDir), formulaName, filepath.Base(rigDir))
				}
			}
		}
	}

	// Check if override exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if formula.EmbeddedFormulaExists(formulaName) {
			return fmt.Errorf("No override found for '%s' at %s level. Already using embedded version.", formulaName, targetLevel)
		}
		return fmt.Errorf("No override found for '%s' at %s level.", formulaName, targetLevel)
	}

	// Remove the override
	if err := os.Remove(targetPath); err != nil {
		return fmt.Errorf("removing override: %w", err)
	}

	fmt.Printf("Removed override from %s level.\n", targetLevel)
	if formula.EmbeddedFormulaExists(formulaName) {
		fmt.Printf("Now using embedded version of '%s'.\n", formulaName)
	} else {
		fmt.Printf("Formula '%s' is no longer available (was custom, not in embedded).\n", formulaName)
	}

	return nil
}

// runFormulaUpdate performs agent-assisted merge of updated embedded formula into override
func runFormulaUpdate(cmd *cobra.Command, args []string) error {
	formulaName := args[0]

	fmt.Printf("Checking for updates to %s...\n\n", formulaName)

	// Verify the formula exists in embedded
	if !formula.EmbeddedFormulaExists(formulaName) {
		return fmt.Errorf("formula '%s' not found in embedded formulas", formulaName)
	}

	townRoot, err := findTownRoot()
	if err != nil {
		return fmt.Errorf("finding town root: %w", err)
	}

	// Find the override file
	overrides := scanFormulaOverridesForName(townRoot, formulaName)
	if len(overrides) == 0 {
		return fmt.Errorf("No override found for '%s'. Nothing to update.\n\nUse 'gt formula modify %s' to create an override first.", formulaName, formulaName)
	}

	// Use the most-specific override (rig > town)
	override := overrides[0]
	for _, o := range overrides {
		if o.Level == "rig" {
			override = o
			break
		}
	}

	// Read the override content
	overrideContent, err := os.ReadFile(override.Path)
	if err != nil {
		return fmt.Errorf("reading override file: %w", err)
	}

	// Extract base hash from override
	baseHash := formula.ExtractBaseHash(overrideContent)

	// Get current embedded hash
	currentHash, err := formula.GetEmbeddedFormulaHash(formulaName)
	if err != nil {
		return fmt.Errorf("computing embedded hash: %w", err)
	}

	// Compare hashes
	if baseHash != "" && baseHash == currentHash {
		fmt.Printf("Override is based on the current embedded version. No update needed.\n")
		return nil
	}

	// Get current embedded content
	embeddedContent, err := formula.GetEmbeddedFormula(formulaName)
	if err != nil {
		return fmt.Errorf("reading embedded formula: %w", err)
	}

	// Print status
	fmt.Printf("Your override: %s\n", override.Path)
	if baseHash != "" {
		fmt.Printf("Based on:      sha256:%s\n", truncateHash(baseHash))
	} else {
		fmt.Printf("Based on:      (unknown - no base version recorded)\n")
	}
	fmt.Printf("Current:       sha256:%s\n\n", truncateHash(currentHash))

	// Detect agent
	agentName, agentCmd, agentArgs, err := detectFormulaUpdateAgent(townRoot)
	if err != nil {
		return fmt.Errorf("detecting agent: %w", err)
	}

	fmt.Printf("Invoking %s to merge changes...\n\n", agentName)

	// Build the merge prompt
	prompt := buildMergePrompt(formulaName, baseHash, currentHash, string(embeddedContent), string(overrideContent))

	// Build the agent command
	fullArgs := append(agentArgs, prompt)
	agentExec := exec.Command(agentCmd, fullArgs...)
	agentExec.Stderr = os.Stderr

	// Capture stdout
	mergedOutput, err := agentExec.Output()
	if err != nil {
		return fmt.Errorf("agent merge failed: %w\n\nYou can manually merge by comparing:\n  Embedded: gt formula show %s\n  Override: %s", err, formulaName, override.Path)
	}

	mergedContent := strings.TrimSpace(string(mergedOutput))
	if mergedContent == "" {
		return fmt.Errorf("agent returned empty output. Manual merge may be required.\n\nCompare:\n  Embedded: gt formula show %s\n  Override: %s", formulaName, override.Path)
	}

	if formulaUpdateApply {
		// Create backup
		bakPath := override.Path + ".bak"
		if err := os.WriteFile(bakPath, overrideContent, 0644); err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
		fmt.Printf("Backup created: %s\n", bakPath)

		// Update the header with new base hash
		header := fmt.Sprintf("# Formula override created by gt formula modify\n# Based on embedded version: sha256:%s\n# To update: gt formula update %s\n\n", currentHash, formulaName)

		// Strip any existing header from merged content
		contentToWrite := stripFormulaHeader(mergedContent)
		finalContent := header + contentToWrite

		if err := os.WriteFile(override.Path, []byte(finalContent), 0644); err != nil {
			return fmt.Errorf("writing merged result: %w", err)
		}

		fmt.Printf("Override updated: %s\n", override.Path)
		fmt.Printf("\nBase version updated to current embedded (sha256:%s).\n", truncateHash(currentHash))
	} else {
		// Output proposed merge to stdout
		fmt.Printf("═══════════════════════════════════════════════════════════\n")
		fmt.Printf("PROPOSED MERGE\n")
		fmt.Printf("═══════════════════════════════════════════════════════════\n\n")
		fmt.Println(mergedContent)
		fmt.Printf("\n═══════════════════════════════════════════════════════════\n\n")
		fmt.Printf("Review the proposed merge above.\n")
		fmt.Printf("Run 'gt formula update %s --apply' to apply it.\n", formulaName)
	}

	return nil
}

// detectFormulaUpdateAgent detects which agent to use for formula merging.
// Returns: agentName, command, args (for one-shot prompt), error
func detectFormulaUpdateAgent(townRoot string) (string, string, []string, error) {
	// 1. Check $GT_DEFAULT_AGENT environment variable
	if envAgent := os.Getenv("GT_DEFAULT_AGENT"); envAgent != "" {
		return resolveAgentForOneShot(envAgent)
	}

	// 2. Try to resolve from town config
	townSettingsPath := filepath.Join(townRoot, "settings", "config.json")
	if data, err := os.ReadFile(townSettingsPath); err == nil {
		content := string(data)
		// Simple extraction of default_agent from JSON
		if idx := strings.Index(content, `"default_agent"`); idx != -1 {
			rest := content[idx:]
			if colonIdx := strings.Index(rest, ":"); colonIdx != -1 {
				valPart := strings.TrimSpace(rest[colonIdx+1:])
				if len(valPart) > 0 && valPart[0] == '"' {
					endQuote := strings.Index(valPart[1:], `"`)
					if endQuote != -1 {
						agentName := valPart[1 : endQuote+1]
						if agentName != "" {
							name, cmd, args, err := resolveAgentForOneShot(agentName)
							if err == nil {
								return name, cmd, args, nil
							}
						}
					}
				}
			}
		}
	}

	// 3. Check if known agents exist on PATH
	agentCandidates := []string{"claude", "opencode", "gemini", "codex"}
	for _, candidate := range agentCandidates {
		if _, err := exec.LookPath(candidate); err == nil {
			return resolveAgentForOneShot(candidate)
		}
	}

	return "", "", nil, fmt.Errorf("no AI agent found.\n\nInstall one of: claude, opencode, gemini, codex\nOr set $GT_DEFAULT_AGENT to your preferred agent.")
}

// resolveAgentForOneShot resolves an agent name to one-shot command invocation details.
// Returns: agentName, command, args (prompt is appended as the last arg), error
func resolveAgentForOneShot(agentName string) (string, string, []string, error) {
	preset := config.GetAgentPresetByName(agentName)
	if preset == nil {
		// Unknown agent - try as a raw command
		if _, err := exec.LookPath(agentName); err != nil {
			return "", "", nil, fmt.Errorf("agent '%s' not found on PATH", agentName)
		}
		// Default to -p for prompt flag (common pattern)
		return agentName, agentName, []string{"-p"}, nil
	}

	// Build one-shot args based on agent's NonInteractive config
	command := preset.Command
	if _, err := exec.LookPath(command); err != nil {
		return "", "", nil, fmt.Errorf("agent '%s' command '%s' not found on PATH", agentName, command)
	}

	var args []string
	if preset.NonInteractive != nil {
		if preset.NonInteractive.Subcommand != "" {
			// e.g., "codex exec" or "opencode run"
			args = append(args, preset.NonInteractive.Subcommand)
		}
		if preset.NonInteractive.PromptFlag != "" {
			// e.g., "-p" for gemini
			args = append(args, preset.NonInteractive.PromptFlag)
		}
	} else {
		// Claude: native non-interactive, just use -p
		args = append(args, "-p")
	}

	return agentName, command, args, nil
}

// buildMergePrompt creates the prompt for the agent to merge formula versions
func buildMergePrompt(formulaName, baseHash, currentHash, embeddedContent, overrideContent string) string {
	var sb strings.Builder

	sb.WriteString("You are merging a formula override with an updated embedded version.\n\n")
	sb.WriteString("TASK: Produce a merged formula that incorporates the upstream changes from the new embedded version while preserving the user's customizations from their override.\n\n")

	sb.WriteString("FORMULA: " + formulaName + "\n\n")

	if baseHash != "" {
		sb.WriteString("The override was originally based on embedded version sha256:" + truncateHash(baseHash) + "\n")
		sb.WriteString("The embedded version has been updated to sha256:" + truncateHash(currentHash) + "\n\n")
	} else {
		sb.WriteString("The override has no recorded base version. Compare it directly against the current embedded version.\n\n")
	}

	sb.WriteString("=== CURRENT EMBEDDED VERSION (new upstream) ===\n")
	sb.WriteString(embeddedContent)
	sb.WriteString("\n=== END EMBEDDED ===\n\n")

	sb.WriteString("=== USER'S OVERRIDE (preserve their customizations) ===\n")
	sb.WriteString(overrideContent)
	sb.WriteString("\n=== END OVERRIDE ===\n\n")

	sb.WriteString("RULES:\n")
	sb.WriteString("1. Preserve all user customizations from the override\n")
	sb.WriteString("2. Incorporate new additions/improvements from the embedded version\n")
	sb.WriteString("3. If there are conflicts, prefer the user's override version\n")
	sb.WriteString("4. Output ONLY the merged TOML content, no explanation or markdown fences\n")
	sb.WriteString("5. Do NOT include the '# Based on embedded version' header comments - those are managed automatically\n")

	return sb.String()
}

// truncateHash returns a short version of a hash for display
func truncateHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// stripFormulaHeader removes the gt-managed header comments from formula content
func stripFormulaHeader(content string) string {
	lines := strings.Split(content, "\n")
	startIdx := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "# Formula override created by") ||
			strings.HasPrefix(trimmed, "# Based on embedded version:") ||
			strings.HasPrefix(trimmed, "# To update: gt formula update") {
			startIdx = i + 1
			continue
		}
		break
	}

	// Skip any blank lines right after the header
	for startIdx < len(lines) && strings.TrimSpace(lines[startIdx]) == "" {
		startIdx++
	}

	if startIdx >= len(lines) {
		return content
	}
	return strings.Join(lines[startIdx:], "\n")
}

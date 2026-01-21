package cmd

import (
	"bufio"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
	"github.com/steveyegge/gastown/internal/workspace"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Formula command flags
var (
	formulaListJSON   bool
	formulaShowJSON   bool
	formulaRunPR      int
	formulaRunRig     string
	formulaRunDryRun  bool
	formulaCreateType string
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

	// Add subcommands
	formulaCmd.AddCommand(formulaListCmd)
	formulaCmd.AddCommand(formulaShowCmd)
	formulaCmd.AddCommand(formulaRunCmd)
	formulaCmd.AddCommand(formulaCreateCmd)

	rootCmd.AddCommand(formulaCmd)
}

// runFormulaList delegates to bd formula list
func runFormulaList(cmd *cobra.Command, args []string) error {
	bdArgs := []string{"formula", "list"}
	if formulaListJSON {
		bdArgs = append(bdArgs, "--json")
	}

	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr
	return bdCmd.Run()
}

// runFormulaShow delegates to bd formula show
func runFormulaShow(cmd *cobra.Command, args []string) error {
	formulaName := args[0]
	bdArgs := []string{"formula", "show", formulaName}
	if formulaShowJSON {
		bdArgs = append(bdArgs, "--json")
	}

	bdCmd := exec.Command("bd", bdArgs...)
	bdCmd.Stdout = os.Stdout
	bdCmd.Stderr = os.Stderr
	return bdCmd.Run()
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
		fmt.Printf("\n  Legs (%d parallel):\n", len(f.Legs))
		for _, leg := range f.Legs {
			fmt.Printf("    • %s: %s\n", leg.ID, leg.Title)
		}
		if f.Synthesis != nil {
			fmt.Printf("\n  Synthesis:\n")
			fmt.Printf("    • %s\n", f.Synthesis.Title)
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

	// Step 2: Create leg beads and track them
	legBeads := make(map[string]string) // leg.ID -> bead ID
	for _, leg := range f.Legs {
		legBeadID := fmt.Sprintf("hq-leg-%s", generateFormulaShortID())

		// Build leg description with prompt if available
		legDesc := leg.Description
		if f.Prompts != nil {
			if basePrompt, ok := f.Prompts["base"]; ok {
				legDesc = fmt.Sprintf("%s\n\n---\nBase Prompt:\n%s", leg.Description, basePrompt)
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

// findFormulaFile searches for a formula file by name
func findFormulaFile(name string) (string, error) {
	// Search paths in order
	searchPaths := []string{}

	// 1. Project .beads/formulas/
	if cwd, err := os.Getwd(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(cwd, ".beads", "formulas"))
	}

	// 2. Town .beads/formulas/
	if townRoot, err := workspace.FindFromCwd(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(townRoot, ".beads", "formulas"))
	}

	// 3. User ~/.beads/formulas/
	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, ".beads", "formulas"))
	}

	// Try each path with common extensions
	extensions := []string{".formula.toml", ".formula.json"}
	for _, basePath := range searchPaths {
		for _, ext := range extensions {
			path := filepath.Join(basePath, name+ext)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	return "", fmt.Errorf("formula '%s' not found in search paths", name)
}

// parseFormulaFile parses a formula file into formulaData
func parseFormulaFile(path string) (*formulaData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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

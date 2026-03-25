package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/style"
)

var (
	rigContractForce         bool
	rigContractRepoType      string
	rigContractTier          string
	rigContractGitHubActions bool
)

var rigContractCmd = &cobra.Command{
	Use:   "contract",
	Short: "Scaffold repo-local safety contract files",
	Long: `Scaffold the committed repo-local safety contract for a rig.

This writes files into the managed repository clone, typically:
  - .gastown/settings.json
  - scripts/ci/verify.sh
  - .github/workflows/ci.yml (optional)

Use this to give Gastown and GitHub Actions one canonical verifier entrypoint
for the repo. Existing files are preserved unless --force is provided.`,
	RunE: requireSubcommand,
}

var rigContractInitCmd = &cobra.Command{
	Use:   "init <rig>",
	Short: "Scaffold repo-local CI contract files for a rig",
	Long: `Scaffold a repo-local verifier contract into the rig's managed repository.

The files are written into the mayor clone so they can be reviewed and committed
like normal repo changes. By default, the command creates:
  - .gastown/settings.json with strict merge-queue verification
  - scripts/ci/verify.sh with stack-aware verifier stubs

If --github-actions is enabled, it also creates .github/workflows/ci.yml when
that file does not already exist. Existing files are not overwritten unless
--force is set.`,
	Args: cobra.ExactArgs(1),
	RunE: runRigContractInit,
}

func init() {
	rigCmd.AddCommand(rigContractCmd)
	rigContractCmd.AddCommand(rigContractInitCmd)

	rigContractInitCmd.Flags().BoolVar(&rigContractForce, "force", false, "Overwrite existing scaffold files")
	rigContractInitCmd.Flags().StringVar(&rigContractRepoType, "repo-type", "", "Repo type override (library, backend-api, frontend-app, worker, cli, data-pipeline, infra)")
	rigContractInitCmd.Flags().StringVar(&rigContractTier, "tier", config.RepoContractTierStrong, "Enforcement tier (basic, strong, production)")
	rigContractInitCmd.Flags().BoolVar(&rigContractGitHubActions, "github-actions", true, "Create a GitHub Actions workflow when one does not already exist")
}

type repoContractScaffoldPlan struct {
	RepoType        string
	EnforcementTier string
	HasGo           bool
	HasNode         bool
	HasPython       bool
	HasTypeScript   bool
	HasFrontend     bool
}

type scaffoldFile struct {
	Path    string
	Content []byte
	Mode    fs.FileMode
}

func runRigContractInit(cmd *cobra.Command, args []string) error {
	rigName := args[0]

	townRoot, r, err := getRig(rigName)
	if err != nil {
		return err
	}

	repoRoot := rigManagedRepoRoot(r.Path)
	if _, statErr := os.Stat(repoRoot); statErr != nil {
		return fmt.Errorf("managed repo not found at %s", repoRoot)
	}

	plan, err := detectRepoContractScaffoldPlan(repoRoot, rigContractRepoType, rigContractTier)
	if err != nil {
		return err
	}

	files, err := buildRigContractScaffoldFiles(repoRoot, plan, rigContractGitHubActions)
	if err != nil {
		return err
	}

	fmt.Printf("Scaffolding repo contract for %s\n", style.Bold.Render(rigName))
	fmt.Printf("  Repo root: %s\n", repoRoot)
	fmt.Printf("  Repo type: %s\n", plan.RepoType)
	fmt.Printf("  Tier: %s\n", plan.EnforcementTier)

	created := 0
	updated := 0
	skipped := 0
	for _, file := range files {
		status, writeErr := writeScaffoldFile(file, rigContractForce)
		if writeErr != nil {
			return writeErr
		}
		switch status {
		case "created":
			created++
			fmt.Printf("  %s created %s\n", style.Success.Render("✓"), file.Path)
		case "updated":
			updated++
			fmt.Printf("  %s updated %s\n", style.Success.Render("✓"), file.Path)
		case "skipped":
			skipped++
			fmt.Printf("  %s kept existing %s\n", style.Dim.Render("○"), file.Path)
		default:
			fmt.Printf("  %s unchanged %s\n", style.Dim.Render("○"), file.Path)
		}
	}

	fmt.Printf("\nSummary: %d created, %d updated, %d kept\n", created, updated, skipped)
	fmt.Printf("Review and commit the repo changes in %s\n", style.Bold.Render(repoRoot))
	if !rigContractForce {
		fmt.Printf("Use %s to overwrite existing scaffold files\n", style.Dim.Render("gt rig contract init "+rigName+" --force"))
	}
	if rigContractGitHubActions {
		fmt.Printf("If the repo already has CI, update it to call %s\n", style.Dim.Render("./scripts/ci/verify.sh"))
	}
	_ = townRoot
	return nil
}

func rigManagedRepoRoot(rigPath string) string {
	repoRoot := filepath.Join(rigPath, "mayor", "rig")
	if _, err := os.Stat(repoRoot); err == nil {
		return repoRoot
	}
	return rigPath
}

func detectRepoContractScaffoldPlan(repoRoot, repoTypeOverride, tierOverride string) (repoContractScaffoldPlan, error) {
	plan := repoContractScaffoldPlan{
		EnforcementTier: strings.TrimSpace(tierOverride),
	}
	if plan.EnforcementTier == "" {
		plan.EnforcementTier = config.RepoContractTierStrong
	}
	if !isSupportedRepoContractTier(plan.EnforcementTier) {
		return plan, fmt.Errorf("invalid enforcement tier %q", plan.EnforcementTier)
	}

	if err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".repo.git", "node_modules", ".venv", "venv", "vendor", "dist", "build", ".next", ".turbo", ".pytest_cache":
				return filepath.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "go.mod":
			plan.HasGo = true
		case "package.json":
			plan.HasNode = true
		case "tsconfig.json":
			plan.HasTypeScript = true
		case "requirements.txt", "pyproject.toml", "setup.py":
			plan.HasPython = true
		case "next.config.js", "next.config.mjs", "next.config.ts", "vite.config.js", "vite.config.ts":
			plan.HasFrontend = true
		}
		return nil
	}); err != nil {
		return plan, fmt.Errorf("scanning repo root %s: %w", repoRoot, err)
	}

	if strings.TrimSpace(repoTypeOverride) != "" {
		plan.RepoType = strings.TrimSpace(repoTypeOverride)
	} else {
		plan.RepoType = inferRepoContractRepoType(plan)
	}
	if !isSupportedRepoContractType(plan.RepoType) {
		return plan, fmt.Errorf("invalid repo type %q", plan.RepoType)
	}
	return plan, nil
}

func isSupportedRepoContractType(repoType string) bool {
	switch repoType {
	case "library", "backend-api", "frontend-app", "worker", "cli", "data-pipeline", "infra":
		return true
	default:
		return false
	}
}

func isSupportedRepoContractTier(tier string) bool {
	switch tier {
	case config.RepoContractTierBasic, config.RepoContractTierStrong, config.RepoContractTierProduction:
		return true
	default:
		return false
	}
}

func inferRepoContractRepoType(plan repoContractScaffoldPlan) string {
	switch {
	case plan.HasFrontend:
		return "frontend-app"
	case plan.HasGo || plan.HasPython || plan.HasNode || plan.HasTypeScript:
		return "backend-api"
	default:
		return "library"
	}
}

func buildRigContractScaffoldFiles(repoRoot string, plan repoContractScaffoldPlan, includeGitHubActions bool) ([]scaffoldFile, error) {
	settings := &config.RigSettings{
		Type:    "rig-settings",
		Version: config.CurrentRigSettingsVersion,
		MergeQueue: &config.MergeQueueConfig{
			VerificationMode: config.VerificationModeStrict,
		},
		RepoContract: &config.RepoContractConfig{
			RepoType:        plan.RepoType,
			EnforcementTier: plan.EnforcementTier,
			VerifyCommand:   "./scripts/ci/verify.sh",
		},
	}

	tempDir, err := os.MkdirTemp("", "gt-rig-contract-*")
	if err != nil {
		return nil, fmt.Errorf("creating temporary scaffold dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	settingsPath := filepath.Join(tempDir, config.RepoSettingsPath)
	if err := config.SaveRigSettings(settingsPath, settings); err != nil {
		return nil, fmt.Errorf("building repo settings scaffold: %w", err)
	}
	settingsContent, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, fmt.Errorf("reading generated repo settings scaffold: %w", err)
	}

	files := []scaffoldFile{
		{
			Path:    filepath.Join(repoRoot, config.RepoSettingsPath),
			Content: settingsContent,
			Mode:    0644,
		},
		{
			Path:    filepath.Join(repoRoot, "scripts", "ci", "verify.sh"),
			Content: []byte(renderRepoVerifierScript(plan)),
			Mode:    0755,
		},
	}

	if includeGitHubActions {
		files = append(files, scaffoldFile{
			Path:    filepath.Join(repoRoot, ".github", "workflows", "ci.yml"),
			Content: []byte(renderGitHubActionsWorkflow(plan)),
			Mode:    0644,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

func writeScaffoldFile(file scaffoldFile, force bool) (string, error) {
	if err := os.MkdirAll(filepath.Dir(file.Path), 0755); err != nil {
		return "", fmt.Errorf("creating directory for %s: %w", file.Path, err)
	}

	existing, err := os.ReadFile(file.Path)
	if err == nil {
		if string(existing) == string(file.Content) {
			if chmodErr := os.Chmod(file.Path, file.Mode); chmodErr == nil {
				return "unchanged", nil
			}
			return "unchanged", nil
		}
		if !force {
			return "skipped", nil
		}
		if writeErr := os.WriteFile(file.Path, file.Content, file.Mode); writeErr != nil {
			return "", fmt.Errorf("writing %s: %w", file.Path, writeErr)
		}
		return "updated", nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading %s: %w", file.Path, err)
	}
	if writeErr := os.WriteFile(file.Path, file.Content, file.Mode); writeErr != nil {
		return "", fmt.Errorf("writing %s: %w", file.Path, writeErr)
	}
	return "created", nil
}

func renderRepoVerifierScript(plan repoContractScaffoldPlan) string {
	lines := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"",
		"ROOT=\"$(cd \"$(dirname \"${BASH_SOURCE[0]}\")/../..\" && pwd)\"",
		"cd \"$ROOT\"",
		"",
		"echo \"[verify] running repo verifier in $ROOT\"",
	}

	if plan.HasGo {
		lines = append(lines,
			"",
			"# Go projects",
			"if [[ -f go.mod ]]; then",
			"  go test ./...",
			"  go vet ./...",
			"fi",
		)
	}

	if plan.HasPython {
		lines = append(lines,
			"",
			"# Python projects",
			"if find . -path '*/.git' -prune -o -path '*/node_modules' -prune -o -path '*/.venv' -prune -o \\( -name requirements.txt -o -name pyproject.toml -o -name setup.py \\) -print -quit | grep -q .; then",
			"  PYTHON_BIN=\"${PYTHON:-python3}\"",
			"  while IFS= read -r -d '' req; do",
			"    \"$PYTHON_BIN\" -m pip install -r \"$req\"",
			"  done < <(find . -path '*/.git' -prune -o -path '*/node_modules' -prune -o -path '*/.venv' -prune -o -name requirements.txt -print0)",
			"  if command -v ruff >/dev/null 2>&1; then",
			"    ruff check .",
			"  else",
			"    \"$PYTHON_BIN\" -m ruff check .",
			"  fi",
			"  if find . -path '*/.git' -prune -o -path '*/node_modules' -prune -o -path '*/tests' -type d -print -quit | grep -q .; then",
			"    \"$PYTHON_BIN\" -m pytest -v",
			"  fi",
			"fi",
		)
	}

	if plan.HasNode {
		lines = append(lines,
			"",
			"# Node / TypeScript projects",
			"while IFS= read -r -d '' package_json; do",
			"  project_dir=\"$(dirname \"$package_json\")\"",
			"  echo \"[verify] node project: $project_dir\"",
			"  if [[ -f \"$project_dir/pnpm-lock.yaml\" || -f \"$ROOT/pnpm-lock.yaml\" ]]; then",
			"    echo \"[verify] TODO: replace npm commands with pnpm for $project_dir if this repo uses pnpm\"",
			"  fi",
			"  if [[ -f \"$project_dir/yarn.lock\" || -f \"$ROOT/yarn.lock\" ]]; then",
			"    echo \"[verify] TODO: replace npm commands with yarn for $project_dir if this repo uses yarn\"",
			"  fi",
			"  (",
			"    cd \"$project_dir\"",
			"    if [[ -f package-lock.json ]]; then",
			"      npm ci",
			"    else",
			"      npm install",
			"    fi",
			"    if [[ -f tsconfig.json ]]; then",
			"      npx tsc --noEmit",
			"    fi",
			"    npm test --if-present",
			"  )",
			"done < <(find . -path '*/.git' -prune -o -path '*/node_modules' -prune -o -name package.json -print0)",
		)
	}

	if !plan.HasGo && !plan.HasPython && !plan.HasNode {
		lines = append(lines,
			"",
			"echo \"[verify] TODO: define repo-specific verification steps in scripts/ci/verify.sh\" >&2",
			"exit 1",
		)
	} else {
		lines = append(lines,
			"",
			"echo \"[verify] ok\"",
		)
	}

	return strings.Join(lines, "\n") + "\n"
}

func renderGitHubActionsWorkflow(plan repoContractScaffoldPlan) string {
	lines := []string{
		"name: CI",
		"",
		"on:",
		"  push:",
		"    branches: [main]",
		"  pull_request:",
		"    branches: [main]",
		"",
		"jobs:",
		"  verify:",
		"    runs-on: ubuntu-latest",
		"    steps:",
		"      - name: Check out repo",
		"        uses: actions/checkout@v4",
	}

	if plan.HasGo {
		lines = append(lines,
			"      - name: Set up Go",
			"        uses: actions/setup-go@v5",
			"        with:",
			"          go-version-file: go.mod",
		)
	}
	if plan.HasPython {
		lines = append(lines,
			"      - name: Set up Python",
			"        uses: actions/setup-python@v5",
			"        with:",
			"          python-version: '3.11'",
		)
	}
	if plan.HasNode {
		lines = append(lines,
			"      - name: Set up Node",
			"        uses: actions/setup-node@v4",
			"        with:",
			"          node-version: '20'",
		)
	}

	lines = append(lines,
		"      - name: Run repo verifier",
		"        run: ./scripts/ci/verify.sh",
	)

	return strings.Join(lines, "\n") + "\n"
}

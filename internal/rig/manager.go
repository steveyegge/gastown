package rig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/claude"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
)

// Common errors
var (
	ErrRigNotFound = errors.New("rig not found")
	ErrRigExists   = errors.New("rig already exists")
)

// wrapCloneError wraps clone errors with helpful suggestions.
// Detects common auth failures and suggests SSH as an alternative.
func wrapCloneError(err error, gitURL string) error {
	errStr := err.Error()

	// Check for GitHub password auth failure
	if strings.Contains(errStr, "Password authentication is not supported") ||
		strings.Contains(errStr, "Authentication failed") {
		// Check if they used HTTPS
		if strings.HasPrefix(gitURL, "https://") {
			// Try to suggest the SSH equivalent
			sshURL := convertToSSH(gitURL)
			if sshURL != "" {
				return fmt.Errorf("creating bare repo: %w\n\nHint: GitHub no longer supports password authentication.\nTry using SSH instead:\n  gt rig add <name> %s", err, sshURL)
			}
			return fmt.Errorf("creating bare repo: %w\n\nHint: GitHub no longer supports password authentication.\nTry using an SSH URL (git@github.com:owner/repo.git) or a personal access token.", err)
		}
	}

	return fmt.Errorf("creating bare repo: %w", err)
}

// convertToSSH converts an HTTPS GitHub/GitLab URL to SSH format.
// Returns empty string if conversion is not possible.
func convertToSSH(httpsURL string) string {
	// Handle GitHub: https://github.com/owner/repo.git -> git@github.com:owner/repo.git
	if strings.HasPrefix(httpsURL, "https://github.com/") {
		path := strings.TrimPrefix(httpsURL, "https://github.com/")
		if !strings.HasSuffix(path, ".git") {
			path += ".git"
		}
		return "git@github.com:" + path
	}

	// Handle GitLab: https://gitlab.com/owner/repo.git -> git@gitlab.com:owner/repo.git
	if strings.HasPrefix(httpsURL, "https://gitlab.com/") {
		path := strings.TrimPrefix(httpsURL, "https://gitlab.com/")
		if !strings.HasSuffix(path, ".git") {
			path += ".git"
		}
		return "git@gitlab.com:" + path
	}

	return ""
}

// RigConfig represents the rig-level configuration (config.json at rig root).
type RigConfig struct {
	Type          string       `json:"type"`                     // "rig"
	Version       int          `json:"version"`                  // schema version
	Name          string       `json:"name"`                     // rig name
	GitURL        string       `json:"git_url"`                  // repository URL
	LocalRepo     string       `json:"local_repo,omitempty"`     // optional local reference repo
	DefaultBranch string       `json:"default_branch,omitempty"` // main, master, etc.
	TargetBranch  string       `json:"target_branch,omitempty"`  // override for merge target (e.g., release/v1.0)
	CreatedAt     time.Time    `json:"created_at"`               // when rig was created
	Beads         *BeadsConfig `json:"beads,omitempty"`
}

// BeadsConfig represents beads configuration for the rig.
type BeadsConfig struct {
	Prefix     string `json:"prefix"`                // issue prefix (e.g., "gt")
	SyncRemote string `json:"sync_remote,omitempty"` // git remote for bd sync
}

// CurrentRigConfigVersion is the current schema version.
const CurrentRigConfigVersion = 1

// Manager handles rig discovery, loading, and creation.
type Manager struct {
	townRoot string
	config   *config.RigsConfig
	git      *git.Git
}

// NewManager creates a new rig manager.
func NewManager(townRoot string, rigsConfig *config.RigsConfig, g *git.Git) *Manager {
	return &Manager{
		townRoot: townRoot,
		config:   rigsConfig,
		git:      g,
	}
}

// DiscoverRigs returns all rigs registered in the workspace.
// Rigs that fail to load are logged to stderr and skipped; partial results are returned.
func (m *Manager) DiscoverRigs() ([]*Rig, error) {
	var rigs []*Rig

	for name, entry := range m.config.Rigs {
		rig, err := m.loadRig(name, entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to load rig %q: %v\n", name, err)
			continue
		}
		rigs = append(rigs, rig)
	}

	return rigs, nil
}

// GetRig returns a specific rig by name.
func (m *Manager) GetRig(name string) (*Rig, error) {
	entry, ok := m.config.Rigs[name]
	if !ok {
		return nil, ErrRigNotFound
	}

	return m.loadRig(name, entry)
}

// RigExists checks if a rig is registered.
func (m *Manager) RigExists(name string) bool {
	_, ok := m.config.Rigs[name]
	return ok
}

// loadRig loads rig details from the filesystem.
func (m *Manager) loadRig(name string, entry config.RigEntry) (*Rig, error) {
	rigPath := filepath.Join(m.townRoot, name)

	// Verify directory exists
	info, err := os.Stat(rigPath)
	if err != nil {
		return nil, fmt.Errorf("rig directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", rigPath)
	}

	rig := &Rig{
		Name:      name,
		Path:      rigPath,
		GitURL:    entry.GitURL,
		LocalRepo: entry.LocalRepo,
		Config:    entry.BeadsConfig,
	}

	// Scan for polecats
	polecatsDir := filepath.Join(rigPath, "polecats")
	if entries, err := os.ReadDir(polecatsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			rig.Polecats = append(rig.Polecats, name)
		}
	}

	// Scan for crew workers
	crewDir := filepath.Join(rigPath, "crew")
	if entries, err := os.ReadDir(crewDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				rig.Crew = append(rig.Crew, e.Name())
			}
		}
	}

	// Check for witness (witnesses don't have clones, just the witness directory)
	witnessPath := filepath.Join(rigPath, "witness")
	if info, err := os.Stat(witnessPath); err == nil && info.IsDir() {
		rig.HasWitness = true
	}

	// Check for refinery
	refineryPath := filepath.Join(rigPath, "refinery", "rig")
	if _, err := os.Stat(refineryPath); err == nil {
		rig.HasRefinery = true
	}

	// Check for mayor clone
	mayorPath := filepath.Join(rigPath, "mayor", "rig")
	if _, err := os.Stat(mayorPath); err == nil {
		rig.HasMayor = true
	}

	return rig, nil
}

// AddRigOptions configures rig creation.
type AddRigOptions struct {
	Name          string // Rig name (directory name)
	GitURL        string // Repository URL
	BeadsPrefix   string // Beads issue prefix (defaults to derived from name)
	LocalRepo     string // Optional local repo for reference clones
	DefaultBranch string // Default branch (defaults to auto-detected from remote)
}

func resolveLocalRepo(path, gitURL string) (string, string) {
	if path == "" {
		return "", ""
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Sprintf("local repo path invalid: %v", err)
	}

	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Sprintf("local repo path invalid: %v", err)
	}

	repoGit := git.NewGit(absPath)
	if !repoGit.IsRepo() {
		return "", fmt.Sprintf("local repo is not a git repository: %s", absPath)
	}

	origin, err := repoGit.RemoteURL("origin")
	if err != nil {
		return absPath, "local repo has no origin; using it anyway"
	}
	if origin != gitURL {
		return "", fmt.Sprintf("local repo origin %q does not match %q", origin, gitURL)
	}

	return absPath, ""
}

// AddRig creates a new rig as a container with clones for each agent.
// The rig structure is:
//
//	<name>/                    # Container (NOT a git clone)
//	├── config.json            # Rig configuration
//	├── .beads/                # Rig-level issue tracking
//	├── refinery/rig/          # Canonical main clone
//	├── mayor/rig/             # Mayor's working clone
//	├── witness/               # Witness agent (no clone)
//	├── polecats/              # Worker directories (empty)
//	└── crew/<crew>/           # Default human workspace
func (m *Manager) AddRig(opts AddRigOptions) (*Rig, error) {
	if m.RigExists(opts.Name) {
		return nil, ErrRigExists
	}

	// Validate rig name: reject characters that break agent ID parsing
	// Agent IDs use format <prefix>-<rig>-<role>[-<name>] with hyphens as delimiters
	if strings.ContainsAny(opts.Name, "-. ") {
		sanitized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(opts.Name)
		sanitized = strings.ToLower(sanitized)
		return nil, fmt.Errorf("rig name %q contains invalid characters; hyphens, dots, and spaces are reserved for agent ID parsing. Try %q instead (underscores are allowed)", opts.Name, sanitized)
	}

	rigPath := filepath.Join(m.townRoot, opts.Name)

	// Check if directory already exists
	if _, err := os.Stat(rigPath); err == nil {
		return nil, fmt.Errorf("directory already exists: %s", rigPath)
	}

	// Track whether user explicitly provided --prefix (before deriving)
	userProvidedPrefix := opts.BeadsPrefix != ""

	// Derive defaults
	if opts.BeadsPrefix == "" {
		opts.BeadsPrefix = deriveBeadsPrefix(opts.Name)
	}

	localRepo, warn := resolveLocalRepo(opts.LocalRepo, opts.GitURL)
	if warn != "" {
		fmt.Printf("  Warning: %s\n", warn)
	}

	// Create container directory
	if err := os.MkdirAll(rigPath, 0755); err != nil {
		return nil, fmt.Errorf("creating rig directory: %w", err)
	}

	// Track cleanup on failure (best-effort cleanup)
	cleanup := func() { _ = os.RemoveAll(rigPath) }
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	// Create rig config
	rigConfig := &RigConfig{
		Type:      "rig",
		Version:   CurrentRigConfigVersion,
		Name:      opts.Name,
		GitURL:    opts.GitURL,
		LocalRepo: localRepo,
		CreatedAt: time.Now(),
		Beads: &BeadsConfig{
			Prefix: opts.BeadsPrefix,
		},
	}
	if err := m.saveRigConfig(rigPath, rigConfig); err != nil {
		return nil, fmt.Errorf("saving rig config: %w", err)
	}

	// Create shared bare repo as source of truth for refinery and polecats.
	// This allows refinery to see polecat branches without pushing to remote.
	// Mayor remains a separate clone (doesn't need branch visibility).
	fmt.Printf("  Cloning repository (this may take a moment)...\n")
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if localRepo != "" {
		if err := m.git.CloneBareWithReference(opts.GitURL, bareRepoPath, localRepo); err != nil {
			fmt.Printf("  Warning: could not use local repo reference: %v\n", err)
			_ = os.RemoveAll(bareRepoPath)
			if err := m.git.CloneBare(opts.GitURL, bareRepoPath); err != nil {
				return nil, wrapCloneError(err, opts.GitURL)
			}
		}
	} else {
		if err := m.git.CloneBare(opts.GitURL, bareRepoPath); err != nil {
			return nil, wrapCloneError(err, opts.GitURL)
		}
	}
	fmt.Printf("   ✓ Created shared bare repo\n")
	bareGit := git.NewGitWithDir(bareRepoPath, "")

	// Determine default branch: use provided value or auto-detect from remote
	var defaultBranch string
	if opts.DefaultBranch != "" {
		defaultBranch = opts.DefaultBranch
	} else {
		// Try to get default branch from remote first, fall back to local detection
		defaultBranch = bareGit.RemoteDefaultBranch()
		if defaultBranch == "" {
			defaultBranch = bareGit.DefaultBranch()
		}
	}
	rigConfig.DefaultBranch = defaultBranch
	// Re-save config with default branch
	if err := m.saveRigConfig(rigPath, rigConfig); err != nil {
		return nil, fmt.Errorf("updating rig config with default branch: %w", err)
	}

	// Create mayor as regular clone (separate from bare repo).
	// Mayor doesn't need to see polecat branches - that's refinery's job.
	// This also allows mayor to stay on the default branch without conflicting with refinery.
	fmt.Printf("  Creating mayor clone...\n")
	mayorRigPath := filepath.Join(rigPath, "mayor", "rig")
	if err := os.MkdirAll(filepath.Dir(mayorRigPath), 0755); err != nil {
		return nil, fmt.Errorf("creating mayor dir: %w", err)
	}
	if localRepo != "" {
		if err := m.git.CloneWithReference(opts.GitURL, mayorRigPath, localRepo); err != nil {
			fmt.Printf("  Warning: could not use local repo reference: %v\n", err)
			_ = os.RemoveAll(mayorRigPath)
			if err := m.git.Clone(opts.GitURL, mayorRigPath); err != nil {
				return nil, fmt.Errorf("cloning for mayor: %w", err)
			}
		}
	} else {
		if err := m.git.Clone(opts.GitURL, mayorRigPath); err != nil {
			return nil, fmt.Errorf("cloning for mayor: %w", err)
		}
	}

	// Checkout the default branch for mayor (clone defaults to remote's HEAD, not our configured branch)
	mayorGit := git.NewGitWithDir("", mayorRigPath)
	if err := mayorGit.Checkout(defaultBranch); err != nil {
		return nil, fmt.Errorf("checking out default branch for mayor: %w", err)
	}
	fmt.Printf("   ✓ Created mayor clone\n")

	// Check if source repo has tracked .beads/ directory.
	// If so, we need to initialize the database (beads.db is gitignored so it doesn't exist after clone).
	sourceBeadsDir := filepath.Join(mayorRigPath, ".beads")
	sourceBeadsDB := filepath.Join(sourceBeadsDir, "beads.db")
	if _, err := os.Stat(sourceBeadsDir); err == nil {
		// Tracked beads exist - try to detect prefix from existing issues
		sourceBeadsConfig := filepath.Join(sourceBeadsDir, "config.yaml")
		if sourcePrefix := detectBeadsPrefixFromConfig(sourceBeadsConfig); sourcePrefix != "" {
			fmt.Printf("  Detected existing beads prefix '%s' from source repo\n", sourcePrefix)
			// Only error on mismatch if user explicitly provided --prefix
			if userProvidedPrefix && opts.BeadsPrefix != sourcePrefix {
				return nil, fmt.Errorf("prefix mismatch: source repo uses '%s' but --prefix '%s' was provided; use --prefix %s to match existing issues", sourcePrefix, opts.BeadsPrefix, sourcePrefix)
			}
			// Use detected prefix (overrides derived prefix)
			opts.BeadsPrefix = sourcePrefix
			rigConfig.Beads.Prefix = sourcePrefix
			// Re-save rig config with detected prefix
			if err := m.saveRigConfig(rigPath, rigConfig); err != nil {
				return nil, fmt.Errorf("updating rig config with detected prefix: %w", err)
			}
		} else {
			// Detection failed (no issues yet) - use derived/provided prefix
			fmt.Printf("  Using prefix '%s' for tracked beads (no existing issues to detect from)\n", opts.BeadsPrefix)
		}

		// Initialize bd database if it doesn't exist.
		// beads.db is gitignored so it won't exist after clone - we need to create it.
		// Use --no-auto-import to configure custom types BEFORE importing (bd-3q6.10).
		if _, err := os.Stat(sourceBeadsDB); os.IsNotExist(err) {
			// Check if town-level beads uses Dolt backend - inherit it
			townBeadsDir := filepath.Join(m.townRoot, ".beads")
			townBackend := beads.DetectBackend(townBeadsDir)
			initArgs := []string{"init", "--prefix", opts.BeadsPrefix, "--no-auto-import"} // opts.BeadsPrefix validated earlier
			if townBackend == "dolt" {
				initArgs = append(initArgs, "--backend", "dolt")
			}
			cmd := exec.Command("bd", initArgs...)
			cmd.Dir = mayorRigPath
			if output, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("  Warning: Could not init bd database: %v (%s)\n", err, strings.TrimSpace(string(output)))
			}
			// Configure custom types for Gas Town (beads v0.46.0+)
			// IMPORTANT: This must run BEFORE any auto-import to avoid validation failures (bd-3q6.10).
			configCmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
			configCmd.Dir = mayorRigPath
			_, _ = configCmd.CombinedOutput() // Ignore errors - older beads don't need this

			// Trigger JSONL import now that custom types are configured.
			syncCmd := exec.Command("bd", "sync")
			syncCmd.Dir = mayorRigPath
			_, _ = syncCmd.CombinedOutput() // Ignore errors - JSONL might not exist yet
		}
	}

	// Create mayor CLAUDE.md (overrides any from cloned repo)
	if err := m.createRoleCLAUDEmd(mayorRigPath, "mayor", opts.Name, ""); err != nil {
		return nil, fmt.Errorf("creating mayor CLAUDE.md: %w", err)
	}

	// Initialize beads at rig level BEFORE creating worktrees.
	// This ensures rig/.beads exists so worktree redirects can point to it.
	fmt.Printf("  Initializing beads database...\n")
	if err := m.initBeads(rigPath, opts.BeadsPrefix); err != nil {
		return nil, fmt.Errorf("initializing beads: %w", err)
	}
	fmt.Printf("   ✓ Initialized beads (prefix: %s)\n", opts.BeadsPrefix)

	// Provision PRIME.md with Gas Town context for all workers in this rig.
	// This is the fallback if SessionStart hook fails - ensures ALL workers
	// (crew, polecats, refinery, witness) have GUPP and essential Gas Town context.
	// PRIME.md is read by bd prime and output to the agent.
	rigBeadsPath := filepath.Join(rigPath, ".beads")
	if err := beads.ProvisionPrimeMD(rigBeadsPath); err != nil {
		fmt.Printf("  Warning: Could not provision PRIME.md: %v\n", err)
	}

	// Create refinery as worktree from bare repo on default branch.
	// Refinery needs to see polecat branches (shared .repo.git) and merges them.
	// Being on the default branch allows direct merge workflow.
	fmt.Printf("  Creating refinery worktree...\n")
	refineryRigPath := filepath.Join(rigPath, "refinery", "rig")
	if err := os.MkdirAll(filepath.Dir(refineryRigPath), 0755); err != nil {
		return nil, fmt.Errorf("creating refinery dir: %w", err)
	}
	if err := bareGit.WorktreeAddExisting(refineryRigPath, defaultBranch); err != nil {
		return nil, fmt.Errorf("creating refinery worktree: %w", err)
	}
	fmt.Printf("   ✓ Created refinery worktree\n")
	// Set up beads redirect for refinery (points to rig-level .beads)
	if err := beads.SetupRedirect(m.townRoot, refineryRigPath); err != nil {
		fmt.Printf("  Warning: Could not set up refinery beads redirect: %v\n", err)
	}
	// Create refinery CLAUDE.md (overrides any from cloned repo)
	if err := m.createRoleCLAUDEmd(refineryRigPath, "refinery", opts.Name, ""); err != nil {
		return nil, fmt.Errorf("creating refinery CLAUDE.md: %w", err)
	}
	// Create refinery hooks for patrol triggering (at refinery/ level, not rig/)
	refineryPath := filepath.Dir(refineryRigPath)
	runtimeConfig := config.LoadRuntimeConfig(rigPath)
	if err := m.createPatrolHooks(refineryPath, runtimeConfig); err != nil {
		fmt.Printf("  Warning: Could not create refinery hooks: %v\n", err)
	}

	// Create empty crew directory with README (crew members added via gt crew add)
	crewPath := filepath.Join(rigPath, "crew")
	if err := os.MkdirAll(crewPath, 0755); err != nil {
		return nil, fmt.Errorf("creating crew dir: %w", err)
	}
	// Create README with instructions
	readmePath := filepath.Join(crewPath, "README.md")
	readmeContent := `# Crew Directory

This directory contains crew worker workspaces.

## Adding a Crew Member

` + "```bash" + `
gt crew add <name>    # Creates crew/<name>/ with a git clone
` + "```" + `

## Crew vs Polecats

- **Crew**: Persistent, user-managed workspaces (never auto-garbage-collected)
- **Polecats**: Transient, witness-managed workers (cleaned up after work completes)

Use crew for your own workspace. Polecats are for batch work dispatch.
`
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return nil, fmt.Errorf("creating crew README: %w", err)
	}

	// Create witness directory (no clone needed)
	witnessPath := filepath.Join(rigPath, "witness")
	if err := os.MkdirAll(witnessPath, 0755); err != nil {
		return nil, fmt.Errorf("creating witness dir: %w", err)
	}
	// Create witness hooks for patrol triggering
	if err := m.createPatrolHooks(witnessPath, runtimeConfig); err != nil {
		fmt.Printf("  Warning: Could not create witness hooks: %v\n", err)
	}

	// Create polecats directory (empty)
	polecatsPath := filepath.Join(rigPath, "polecats")
	if err := os.MkdirAll(polecatsPath, 0755); err != nil {
		return nil, fmt.Errorf("creating polecats dir: %w", err)
	}

	// Install Claude settings for all agent directories.
	// Settings are placed in parent directories (not inside git repos) so Claude
	// finds them via directory traversal without polluting source repos.
	fmt.Printf("  Installing Claude settings...\n")
	settingsRoles := []struct {
		dir  string
		role string
	}{
		{witnessPath, "witness"},
		{filepath.Join(rigPath, "refinery"), "refinery"},
		{crewPath, "crew"},
		{polecatsPath, "polecat"},
	}
	for _, sr := range settingsRoles {
		if err := claude.EnsureSettingsForRole(sr.dir, sr.role); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: Could not create %s settings: %v\n", sr.role, err)
		}
	}
	fmt.Printf("   ✓ Installed Claude settings\n")

	// Initialize beads at rig level
	fmt.Printf("  Initializing beads database...\n")
	if err := m.initBeads(rigPath, opts.BeadsPrefix); err != nil {
		return nil, fmt.Errorf("initializing beads: %w", err)
	}
	fmt.Printf("   ✓ Initialized beads (prefix: %s)\n", opts.BeadsPrefix)

	// Create rig-level agent beads (witness, refinery) in rig beads.
	// Town-level agents (mayor, deacon) are created by gt install in town beads.
	if err := m.initAgentBeads(rigPath, opts.Name, opts.BeadsPrefix); err != nil {
		// Non-fatal: log warning but continue
		fmt.Fprintf(os.Stderr, "  Warning: Could not create agent beads: %v\n", err)
	}

	// Seed patrol molecules for this rig
	if err := m.seedPatrolMolecules(rigPath); err != nil {
		// Non-fatal: log warning but continue
		fmt.Fprintf(os.Stderr, "  Warning: Could not seed patrol molecules: %v\n", err)
	}

	// Create plugin directories
	if err := m.createPluginDirectories(rigPath); err != nil {
		// Non-fatal: log warning but continue
		fmt.Fprintf(os.Stderr, "  Warning: Could not create plugin directories: %v\n", err)
	}

	// Register in town config
	m.config.Rigs[opts.Name] = config.RigEntry{
		GitURL:    opts.GitURL,
		LocalRepo: localRepo,
		AddedAt:   time.Now(),
		BeadsConfig: &config.BeadsConfig{
			Prefix: opts.BeadsPrefix,
		},
	}

	success = true
	return m.loadRig(opts.Name, m.config.Rigs[opts.Name])
}

// saveRigConfig writes the rig configuration to config.json.
func (m *Manager) saveRigConfig(rigPath string, cfg *RigConfig) error {
	configPath := filepath.Join(rigPath, "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0644)
}

// LoadRigConfig reads the rig configuration from config.json.
func LoadRigConfig(rigPath string) (*RigConfig, error) {
	configPath := filepath.Join(rigPath, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}
	var cfg RigConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// initBeads initializes the beads database at rig level.
// The project's .beads/config.yaml determines sync-branch settings.
// Use `bd doctor --fix` in the project to configure sync-branch if needed.
// TODO(bd-yaml): beads config should migrate to JSON (see beads issue)
func (m *Manager) initBeads(rigPath, prefix string) error {
	// Validate prefix format to prevent command injection from config files
	if !isValidBeadsPrefix(prefix) {
		return fmt.Errorf("invalid beads prefix %q: must be alphanumeric with optional hyphens, start with letter, max 20 chars", prefix)
	}

	beadsDir := filepath.Join(rigPath, ".beads")
	mayorRigBeads := filepath.Join(rigPath, "mayor", "rig", ".beads")

	// Check if town-level uses dolt-native mode (shared database via symlinks).
	townBeadsDir := filepath.Join(m.townRoot, ".beads")
	townIsDoltNative := beads.IsDoltNative(townBeadsDir)

	// Check if source repo has tracked .beads/ (cloned into mayor/rig).
	// If so, create a redirect file instead of a new database.
	if _, err := os.Stat(mayorRigBeads); err == nil {
		// Tracked beads exist

		// If town is dolt-native, set up symlinks to shared database.
		// This keeps tracked config/formulas but shares the dolt database.
		if townIsDoltNative {
			if err := beads.SetupDoltNativeSymlinks(mayorRigBeads, townBeadsDir); err != nil {
				fmt.Printf("   Warning: Could not setup dolt-native symlinks: %v\n", err)
				// Continue - rig will work but with isolated database
			}
		}

		// Create redirect to mayor/rig/.beads
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			return err
		}
		redirectPath := filepath.Join(beadsDir, "redirect")
		if err := os.WriteFile(redirectPath, []byte("mayor/rig/.beads\n"), 0644); err != nil {
			return fmt.Errorf("creating redirect file: %w", err)
		}
		return nil
	}

	// Check if town-level uses Dolt server mode (centralized database).
	// If so, create a redirect to town-level .beads instead of local database.
	// Note: townBeadsDir was declared earlier for dolt-native check.
	if beads.IsDoltServerMode(townBeadsDir) {
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			return err
		}
		// Calculate relative path from rig to town-level .beads
		// rigPath is like ~/gt/gastown, town is ~/gt, so relative is ../.beads
		relPath, err := filepath.Rel(rigPath, townBeadsDir)
		if err != nil {
			relPath = "../.beads" // Fallback to common case
		}
		redirectPath := filepath.Join(beadsDir, "redirect")
		if err := os.WriteFile(redirectPath, []byte(relPath+"\n"), 0644); err != nil {
			return fmt.Errorf("creating redirect file: %w", err)
		}
		return nil
	}

	// No tracked beads and no server mode - create local database
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return err
	}

	// Build environment with explicit BEADS_DIR to prevent bd from
	// finding a parent directory's .beads/ database
	env := os.Environ()
	filteredEnv := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, "BEADS_DIR=") {
			filteredEnv = append(filteredEnv, e)
		}
	}
	filteredEnv = append(filteredEnv, "BEADS_DIR="+beadsDir)

	// Check if town-level beads uses Dolt backend - inherit it for new rigs
	// Note: townBeadsDir is declared earlier for dolt-native check
	townBackend := beads.DetectBackend(townBeadsDir)

	// Run bd init with --no-auto-import to create database WITHOUT importing from JSONL.
	// This allows us to configure custom types BEFORE the import runs (bd-3q6.10).
	// Without this, auto-import fails when issues.jsonl contains custom types like merge-request.
	initArgs := []string{"init", "--prefix", prefix, "--no-auto-import"}
	if townBackend == "dolt" {
		initArgs = append(initArgs, "--backend", "dolt")
	}
	cmd := exec.Command("bd", initArgs...)
	cmd.Dir = rigPath
	cmd.Env = filteredEnv
	_, err := cmd.CombinedOutput()
	if err != nil {
		// bd might not be installed or failed, create minimal structure
		// Note: beads currently expects YAML format for config
		configPath := filepath.Join(beadsDir, "config.yaml")
		configContent := fmt.Sprintf("prefix: %s\n", prefix)
		if writeErr := os.WriteFile(configPath, []byte(configContent), 0644); writeErr != nil {
			return writeErr
		}
	}

	// Configure custom types for Gas Town (agent, role, rig, convoy, merge-request, etc).
	// These were extracted from beads core in v0.46.0 and now require explicit config.
	// IMPORTANT: This must run BEFORE any auto-import to avoid validation failures (bd-3q6.10).
	configCmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
	configCmd.Dir = rigPath
	configCmd.Env = filteredEnv
	// Ignore errors - older beads versions don't need this
	_, _ = configCmd.CombinedOutput()

	// Trigger JSONL import now that custom types are configured.
	// bd sync will import from issues.jsonl and validate types correctly.
	syncCmd := exec.Command("bd", "sync")
	syncCmd.Dir = rigPath
	syncCmd.Env = filteredEnv
	_, _ = syncCmd.CombinedOutput() // Ignore errors - JSONL might not exist yet

	// Ensure database has repository fingerprint (GH #25).
	// This is idempotent - safe on both new and legacy (pre-0.17.5) databases.
	// Without fingerprint, the bd daemon fails to start silently.
	migrateCmd := exec.Command("bd", "migrate", "--update-repo-id")
	migrateCmd.Dir = rigPath
	migrateCmd.Env = filteredEnv
	// Ignore errors - fingerprint is optional for functionality
	_, _ = migrateCmd.CombinedOutput()

	// Ensure issues.jsonl exists to prevent bd auto-export from corrupting other files.
	// bd init creates beads.db but not issues.jsonl in SQLite mode.
	// Without issues.jsonl, bd's auto-export might write issues to other .jsonl files.
	issuesJSONL := filepath.Join(beadsDir, "issues.jsonl")
	if _, err := os.Stat(issuesJSONL); os.IsNotExist(err) {
		if err := os.WriteFile(issuesJSONL, []byte{}, 0644); err != nil {
			// Non-fatal but log it
			fmt.Printf("   ⚠ Could not create issues.jsonl: %v\n", err)
		}
	}

	// NOTE: We intentionally do NOT create routes.jsonl in rig beads.
	// bd's routing walks up to find town root (via mayor/town.json) and uses
	// town-level routes.jsonl for prefix-based routing. Rig-level routes.jsonl
	// would prevent this walk-up and break cross-rig routing.

	return nil
}

// initAgentBeads creates rig-level agent beads for Witness and Refinery.
// These agents use the rig's beads prefix and are stored in rig beads.
//
// Town-level agents (Mayor, Deacon) are created by gt install in town beads.
// Role beads are also created by gt install with hq- prefix.
//
// Rig-level agents (Witness, Refinery) are created here in rig beads with rig prefix.
// Format: <prefix>-<rig>-<role> (e.g., pi-pixelforge-witness)
//
// Agent beads track lifecycle state for ZFC compliance (gt-h3hak, gt-pinkq).
func (m *Manager) initAgentBeads(rigPath, rigName, prefix string) error {
	// Rig-level agents go in rig beads with rig prefix (per docs/architecture.md).
	// Town-level agents (Mayor, Deacon) are created by gt install in town beads.
	// Use ResolveBeadsDir to follow redirect files for tracked beads.
	rigBeadsDir := beads.ResolveBeadsDir(rigPath)
	bd := beads.NewWithBeadsDir(rigPath, rigBeadsDir)

	// Define rig-level agents to create
	type agentDef struct {
		id       string
		roleType string
		rig      string
		desc     string
	}

	// Create rig-specific agents using rig prefix in rig beads.
	// Format: <prefix>-<rig>-<role> (e.g., pi-pixelforge-witness)
	agents := []agentDef{
		{
			id:       beads.WitnessBeadIDWithPrefix(prefix, rigName),
			roleType: "witness",
			rig:      rigName,
			desc:     fmt.Sprintf("Witness for %s - monitors polecat health and progress.", rigName),
		},
		{
			id:       beads.RefineryBeadIDWithPrefix(prefix, rigName),
			roleType: "refinery",
			rig:      rigName,
			desc:     fmt.Sprintf("Refinery for %s - processes merge queue.", rigName),
		},
	}

	// Note: Mayor and Deacon are now created by gt install in town beads.

	for _, agent := range agents {
		// Check if already exists
		if _, err := bd.Show(agent.id); err == nil {
			continue // Already exists
		}

		// Note: RoleBead field removed - role definitions are now config-based
		fields := &beads.AgentFields{
			RoleType:   agent.roleType,
			Rig:        agent.rig,
			AgentState: "idle",
			HookBead:   "",
		}

		if _, err := bd.CreateAgentBead(agent.id, agent.desc, fields); err != nil {
			return fmt.Errorf("creating %s: %w", agent.id, err)
		}
		fmt.Printf("   ✓ Created agent bead: %s\n", agent.id)
	}

	return nil
}

// ensureGitignoreEntry adds an entry to .gitignore if it doesn't already exist.
func (m *Manager) ensureGitignoreEntry(gitignorePath, entry string) error {
	// Read existing content
	content, err := os.ReadFile(gitignorePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Check if entry already exists
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == entry {
			return nil // Already present
		}
	}

	// Append entry
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) //nolint:gosec // G302: .gitignore should be readable by git tools
	if err != nil {
		return err
	}
	defer f.Close()

	// Add newline before if file doesn't end with one
	if len(content) > 0 && content[len(content)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(entry + "\n")
	return err
}

// deriveBeadsPrefix generates a beads prefix from a rig name.
// Examples: "gastown" -> "gt", "my-project" -> "mp", "foo" -> "foo"
func deriveBeadsPrefix(name string) string {
	// Remove common suffixes
	name = strings.TrimSuffix(name, "-py")
	name = strings.TrimSuffix(name, "-go")

	// Split on hyphens/underscores
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})

	// If single part, try to detect compound words (e.g., "gastown" -> "gas" + "town")
	if len(parts) == 1 {
		parts = splitCompoundWord(parts[0])
	}

	if len(parts) >= 2 {
		// Take first letter of each part: "gas-town" -> "gt"
		prefix := ""
		for _, p := range parts {
			if len(p) > 0 {
				prefix += string(p[0])
			}
		}
		return strings.ToLower(prefix)
	}

	// Single word: use first 2-3 chars
	if len(name) <= 3 {
		return strings.ToLower(name)
	}
	return strings.ToLower(name[:2])
}

// splitCompoundWord attempts to split a compound word into its components.
// Common suffixes like "town", "ville", "port" are detected to split
// compound names (e.g., "gastown" -> ["gas", "town"]).
func splitCompoundWord(word string) []string {
	word = strings.ToLower(word)

	// Common suffixes for compound place names
	suffixes := []string{"town", "ville", "port", "place", "land", "field", "wood", "ford"}

	for _, suffix := range suffixes {
		if strings.HasSuffix(word, suffix) && len(word) > len(suffix) {
			prefix := word[:len(word)-len(suffix)]
			if len(prefix) > 0 {
				return []string{prefix, suffix}
			}
		}
	}

	return []string{word}
}

// detectBeadsPrefixFromConfig reads the issue prefix from a beads config.yaml file.
// Returns empty string if the file doesn't exist or doesn't contain a prefix.
// Falls back to detecting prefix from existing issues in issues.jsonl.
//
// beadsPrefixRegexp validates beads prefix format: alphanumeric, may contain hyphens,
// must start with letter, max 20 chars. Prevents shell injection via config files.
var beadsPrefixRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,19}$`)

// isValidBeadsPrefix checks if a prefix is safe for use in shell commands.
// Prefixes must be alphanumeric (with optional hyphens), start with a letter,
// and be at most 20 characters. This prevents command injection from
// malicious config files.
func isValidBeadsPrefix(prefix string) bool {
	return beadsPrefixRegexp.MatchString(prefix)
}

// When adding a rig from a source repo that has .beads/ tracked in git (like a project
// that already uses beads for issue tracking), we need to use that project's existing
// prefix instead of generating a new one. Otherwise, the rig would have a mismatched
// prefix and routing would fail to find the existing issues.
func detectBeadsPrefixFromConfig(configPath string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}

	// Parse YAML-style config (simple line-by-line parsing)
	// Looking for "issue-prefix: <value>" or "prefix: <value>"
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Check for issue-prefix or prefix key
		for _, key := range []string{"issue-prefix:", "prefix:"} {
			if strings.HasPrefix(line, key) {
				value := strings.TrimSpace(strings.TrimPrefix(line, key))
				// Remove quotes if present
				value = strings.Trim(value, `"'`)
				if value != "" && isValidBeadsPrefix(value) {
					return value
				}
			}
		}
	}

	// Fallback: try to detect prefix from existing issues in issues.jsonl
	// Look for the first issue ID pattern like "gt-abc123"
	beadsDir := filepath.Dir(configPath)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	if issuesData, err := os.ReadFile(issuesPath); err == nil {
		issuesLines := strings.Split(string(issuesData), "\n")
		for _, line := range issuesLines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Look for "id":"<prefix>-<hash>" pattern
			if idx := strings.Index(line, `"id":"`); idx != -1 {
				start := idx + 6 // len(`"id":"`)
				if end := strings.Index(line[start:], `"`); end != -1 {
					issueID := line[start : start+end]
					// Extract prefix (everything before the last hyphen-hash part)
					if dashIdx := strings.LastIndex(issueID, "-"); dashIdx > 0 {
						prefix := issueID[:dashIdx]
						// Handle prefixes like "gt" (from "gt-abc") - return without trailing hyphen
						if isValidBeadsPrefix(prefix) {
							return prefix
						}
					}
				}
			}
			break // Only check first issue
		}
	}

	return ""
}

// RemoveRig unregisters a rig (does not delete files).
func (m *Manager) RemoveRig(name string) error {
	if !m.RigExists(name) {
		return ErrRigNotFound
	}

	delete(m.config.Rigs, name)
	return nil
}

// ListRigNames returns the names of all registered rigs.
func (m *Manager) ListRigNames() []string {
	names := make([]string, 0, len(m.config.Rigs))
	for name := range m.config.Rigs {
		names = append(names, name)
	}
	return names
}

// createRoleCLAUDEmd creates a minimal bootstrap pointer CLAUDE.md file.
// Full context is injected ephemerally by `gt prime` at session start.
// This keeps on-disk files small (<30 lines) per the priming architecture.
func (m *Manager) createRoleCLAUDEmd(workspacePath string, role string, rigName string, workerName string) error {
	// Create role-specific bootstrap pointer
	var bootstrap string
	switch role {
	case "mayor":
		bootstrap = `# Mayor Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.
`
	case "refinery":
		bootstrap = `# Refinery Context (` + rigName + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check MQ: ` + "`gt mq list`" + `
- Process next: ` + "`gt mq process`" + `
`
	case "crew":
		name := workerName
		if name == "" {
			name = "worker"
		}
		bootstrap = `# Crew Context (` + rigName + `/` + name + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check hook: ` + "`gt hook`" + `
- Check mail: ` + "`gt mail inbox`" + `
`
	case "polecat":
		name := workerName
		if name == "" {
			name = "worker"
		}
		bootstrap = `# Polecat Context (` + rigName + `/` + name + `)

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.

## Quick Reference

- Check hook: ` + "`gt hook`" + `
- Report done: ` + "`gt done`" + `
`
	default:
		bootstrap = `# Agent Context

> **Recovery**: Run ` + "`gt prime`" + ` after compaction, clear, or new session

Full context is injected by ` + "`gt prime`" + ` at session start.
`
	}

	claudePath := filepath.Join(workspacePath, "CLAUDE.md")
	return os.WriteFile(claudePath, []byte(bootstrap), 0644)
}

// createPatrolHooks creates .claude/settings.json with hooks for patrol roles.
// These hooks trigger gt prime on session start and inject mail, enabling
// autonomous patrol execution for Witness and Refinery roles.
func (m *Manager) createPatrolHooks(workspacePath string, runtimeConfig *config.RuntimeConfig) error {
	if runtimeConfig == nil || runtimeConfig.Hooks == nil || runtimeConfig.Hooks.Provider != "claude" {
		return nil
	}
	if runtimeConfig.Hooks.Dir == "" || runtimeConfig.Hooks.SettingsFile == "" {
		return nil
	}

	settingsDir := filepath.Join(workspacePath, runtimeConfig.Hooks.Dir)
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return fmt.Errorf("creating settings dir: %w", err)
	}

	// Standard patrol hooks - same as deacon
	hooksJSON := `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt prime && gt mail check --inject"
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt prime"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "gt mail check --inject"
          }
        ]
      }
    ]
  }
}
`
	settingsPath := filepath.Join(settingsDir, runtimeConfig.Hooks.SettingsFile)
	return os.WriteFile(settingsPath, []byte(hooksJSON), 0600)
}

// seedPatrolMolecules creates patrol molecule prototypes in the rig's beads database.
// These molecules define the work loops for Deacon, Witness, and Refinery roles.
func (m *Manager) seedPatrolMolecules(rigPath string) error {
	// Use bd command to seed molecules (more reliable than internal API)
	cmd := exec.Command("bd", "mol", "seed", "--patrol")
	cmd.Dir = rigPath
	if err := cmd.Run(); err != nil {
		// Fallback: bd mol seed might not support --patrol yet
		// Try creating them individually via bd create
		return m.seedPatrolMoleculesManually(rigPath)
	}
	return nil
}

// seedPatrolMoleculesManually creates patrol molecules using bd create commands.
func (m *Manager) seedPatrolMoleculesManually(rigPath string) error {
	// Patrol molecule definitions for seeding
	patrolMols := []struct {
		title string
		desc  string
	}{
		{
			title: "Deacon Patrol",
			desc:  "Mayor's daemon patrol loop for handling callbacks, health checks, and cleanup.",
		},
		{
			title: "Witness Patrol",
			desc:  "Per-rig worker monitor patrol loop with progressive nudging.",
		},
		{
			title: "Refinery Patrol",
			desc:  "Merge queue processor patrol loop with verification gates.",
		},
	}

	for _, mol := range patrolMols {
		// Check if already exists by title
		checkCmd := exec.Command("bd", "list", "--type=molecule", "--format=json")
		checkCmd.Dir = rigPath
		output, _ := checkCmd.Output()
		if strings.Contains(string(output), mol.title) {
			continue // Already exists
		}

		// Create the molecule
		cmd := exec.Command("bd", "create", //nolint:gosec // G204: bd is a trusted internal tool
			"--type=molecule",
			"--title="+mol.title,
			"--description="+mol.desc,
			"--priority=2",
		)
		cmd.Dir = rigPath
		if err := cmd.Run(); err != nil {
			// Non-fatal, continue with others
			continue
		}
	}
	return nil
}

// createPluginDirectories creates plugin directories at town and rig levels.
// - ~/gt/plugins/ (town-level, shared across all rigs)
// - <rig>/plugins/ (rig-level, rig-specific plugins)
func (m *Manager) createPluginDirectories(rigPath string) error {
	// Town-level plugins directory
	townPluginsDir := filepath.Join(m.townRoot, "plugins")
	if err := os.MkdirAll(townPluginsDir, 0755); err != nil {
		return fmt.Errorf("creating town plugins directory: %w", err)
	}

	// Create a README in town plugins if it doesn't exist
	townReadme := filepath.Join(townPluginsDir, "README.md")
	if _, err := os.Stat(townReadme); os.IsNotExist(err) {
		content := `# Gas Town Plugins

This directory contains town-level plugins that run during Deacon patrol cycles.

## Plugin Structure

Each plugin is a directory containing:
- plugin.md - Plugin definition with TOML frontmatter

## Gate Types

- cooldown: Time since last run (e.g., 24h)
- cron: Schedule-based (e.g., "0 9 * * *")
- condition: Metric threshold
- event: Trigger-based (startup, heartbeat)

See docs/deacon-plugins.md for full documentation.
`
		if writeErr := os.WriteFile(townReadme, []byte(content), 0644); writeErr != nil {
			// Non-fatal
			return nil
		}
	}

	// Rig-level plugins directory
	rigPluginsDir := filepath.Join(rigPath, "plugins")
	if err := os.MkdirAll(rigPluginsDir, 0755); err != nil {
		return fmt.Errorf("creating rig plugins directory: %w", err)
	}

	// Add plugins/ and .repo.git/ to rig .gitignore
	gitignorePath := filepath.Join(rigPath, ".gitignore")
	if err := m.ensureGitignoreEntry(gitignorePath, "plugins/"); err != nil {
		return err
	}
	return m.ensureGitignoreEntry(gitignorePath, ".repo.git/")
}

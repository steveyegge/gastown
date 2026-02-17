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
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/doltserver"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/templates/commands"
)

// Common errors
var (
	ErrRigNotFound = errors.New("rig not found")
	ErrRigExists   = errors.New("rig already exists")
)

// reservedRigNames are names that cannot be used for rigs because they
// collide with town-level infrastructure. "hq" is special-cased by
// EnsureMetadata and dolt routing as the town-level beads alias.
var reservedRigNames = []string{"hq"}

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
	CreatedAt     time.Time    `json:"created_at"`               // when rig was created
	Beads         *BeadsConfig `json:"beads,omitempty"`
}

// BeadsConfig represents beads configuration for the rig.
type BeadsConfig struct {
	Prefix string `json:"prefix"` // issue prefix (e.g., "gt")
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
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
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

	// Reject reserved names that collide with town-level infrastructure.
	// "hq" is special-cased by EnsureMetadata and dolt routing as the town-level alias.
	for _, reserved := range reservedRigNames {
		if strings.EqualFold(opts.Name, reserved) {
			return nil, fmt.Errorf("rig name %q is reserved for town-level infrastructure", opts.Name)
		}
	}

	rigPath := filepath.Join(m.townRoot, opts.Name)

	// Check if directory already exists
	if _, err := os.Stat(rigPath); err == nil {
		return nil, fmt.Errorf("directory already exists: %s\n\nTo adopt an existing directory, use --adopt:\n  gt rig add %s --adopt", rigPath, opts.Name)
	}

	// Track whether user explicitly provided --prefix (before deriving)
	userProvidedPrefix := opts.BeadsPrefix != ""
	opts.BeadsPrefix = strings.TrimSuffix(opts.BeadsPrefix, "-")

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
	// Validate user-provided branch exists on remote (auto-detected branches are inherently valid)
	if opts.DefaultBranch != "" {
		ref := fmt.Sprintf("origin/%s", defaultBranch)
		if exists, err := bareGit.RefExists(ref); err != nil {
			return nil, fmt.Errorf("checking ref %s: %w", ref, err)
		} else if !exists {
			return nil, fmt.Errorf("branch %q does not exist on remote (ref %s not found in bare repo)", defaultBranch, ref)
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
	// If so, we need to initialize the database (it doesn't exist after clone since DB files are gitignored).
	sourceBeadsDir := filepath.Join(mayorRigPath, ".beads")
	if _, err := os.Stat(sourceBeadsDir); err == nil {
		// Remove any redirect file that might have been accidentally tracked.
		// Redirect files are runtime/local config and should not be in git.
		// If not removed, they can cause circular redirect warnings during rig setup.
		sourceRedirectFile := filepath.Join(sourceBeadsDir, "redirect")
		_ = os.Remove(sourceRedirectFile) // Ignore error if doesn't exist

		// Tracked beads exist - try to detect prefix from existing issues
		sourceBeadsConfig := filepath.Join(sourceBeadsDir, "config.yaml")
		if sourcePrefix := detectBeadsPrefixFromConfig(sourceBeadsConfig); sourcePrefix != "" {
			fmt.Printf("  Detected existing beads prefix '%s' from source repo\n", sourcePrefix)
			// Only error on mismatch if user explicitly provided --prefix
			if userProvidedPrefix && strings.TrimSuffix(opts.BeadsPrefix, "-") != strings.TrimSuffix(sourcePrefix, "-") {
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

		// Initialize bd database if runtime files are missing.
		// DB files are gitignored so they won't exist after clone — bd init creates them.
		// bd init --prefix will create the database and auto-import from issues.jsonl.
		//
		// Note: bdDatabaseExists checks for metadata.json which may be tracked in git.
		// When metadata.json exists but the Dolt server database doesn't (fresh clone
		// to a new workspace), we still need to run bd init to create the server-side
		// database and set issue_prefix. Always ensure issue_prefix is set afterward.
		if !bdDatabaseExists(sourceBeadsDir) {
			cmd := exec.Command("bd", "init", "--prefix", opts.BeadsPrefix, "--server") // opts.BeadsPrefix validated earlier
			cmd.Dir = mayorRigPath
			if output, err := cmd.CombinedOutput(); err != nil {
				fmt.Printf("  Warning: Could not init bd database: %v (%s)\n", err, strings.TrimSpace(string(output)))
			}
		}

		// Always ensure issue_prefix and custom types are configured, even when
		// metadata.json was tracked in git (bdDatabaseExists returned true).
		// The tracked metadata.json tells bd HOW to connect but doesn't guarantee
		// the server-side database has issue_prefix set for this workspace.
		configCmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
		configCmd.Dir = mayorRigPath
		_, _ = configCmd.CombinedOutput() // Ignore errors - older beads don't need this

		prefixSetCmd := exec.Command("bd", "config", "set", "issue_prefix", opts.BeadsPrefix)
		prefixSetCmd.Dir = mayorRigPath
		if prefixOutput, prefixErr := prefixSetCmd.CombinedOutput(); prefixErr != nil {
			fmt.Printf("  Warning: Could not set issue_prefix: %v (%s)\n", prefixErr, strings.TrimSpace(string(prefixOutput)))
		}
	}

	// NOTE: No per-directory CLAUDE.md/AGENTS.md is created for any agent.
	// Only ~/gt/CLAUDE.md (town-root identity anchor) exists on disk.
	// Full context is injected ephemerally by `gt prime` at session start.

	// Create server-side database for this rig BEFORE initializing beads.
	// InitBeads runs bd init --server which writes metadata.json, but the actual
	// database in .dolt-data/ must exist first for bd config commands to work.
	if _, err := exec.LookPath("dolt"); err == nil {
		if _, _, err := doltserver.InitRig(m.townRoot, opts.Name); err != nil {
			fmt.Printf("  Warning: Could not create rig database: %v\n", err)
		}
	}

	// Initialize beads at rig level BEFORE creating worktrees.
	// This ensures rig/.beads exists so worktree redirects can point to it.
	fmt.Printf("  Initializing beads database...\n")
	if err := m.InitBeads(rigPath, opts.BeadsPrefix); err != nil {
		return nil, fmt.Errorf("initializing beads: %w", err)
	}
	fmt.Printf("   ✓ Initialized beads (prefix: %s)\n", opts.BeadsPrefix)

	// Ensure metadata.json has dolt_mode=server and dolt_database=<rigName>.
	// bd init --server sets dolt_mode but not dolt_database. EnsureMetadata
	// writes both fields so bd connects to the correct centralized database.
	// This must happen BEFORE setting issue_prefix below, so bd connects to
	// the correct server-side database (rigName, not beads_<prefix>).
	if err := doltserver.EnsureMetadata(m.townRoot, opts.Name); err != nil {
		// Non-fatal: daemon's EnsureAllMetadata self-heals on next startup,
		// or user can run gt doctor --fix to repair manually.
		fmt.Printf("  Warning: Could not set Dolt server metadata: %v\n", err)
		fmt.Printf("  Run 'gt doctor --fix' to repair, or it will self-heal on next daemon start.\n")
	}

	// Set issue_prefix on the correct server-side database.
	// InitBeads ran bd config set issue_prefix, but against the wrong database
	// (beads_<prefix> from bd init, not <rigName> from the centralized server).
	// Now that EnsureMetadata has corrected dolt_database, re-set it.
	{
		resolvedBeadsDir := beads.ResolveBeadsDir(rigPath)
		prefixCmd := exec.Command("bd", "config", "set", "issue_prefix", opts.BeadsPrefix)
		prefixCmd.Dir = rigPath
		prefixCmd.Env = append(os.Environ(), "BEADS_DIR="+resolvedBeadsDir)
		if out, err := prefixCmd.CombinedOutput(); err != nil {
			fmt.Printf("  Warning: Could not set issue_prefix on rig database: %v (%s)\n", err, strings.TrimSpace(string(out)))
		}
		typesCmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
		typesCmd.Dir = rigPath
		typesCmd.Env = append(os.Environ(), "BEADS_DIR="+resolvedBeadsDir)
		_, _ = typesCmd.CombinedOutput()
	}

	// Auto-create DoltHub remote for the rig's beads database.
	// Requires DOLTHUB_TOKEN and DOLTHUB_ORG environment variables.
	// Non-fatal: sync will work without a remote; user can add one manually later.
	if token := doltserver.DoltHubToken(); token != "" {
		if org := doltserver.DoltHubOrg(); org != "" {
			dbName := "beads_" + opts.Name
			dbDir := doltserver.RigDatabaseDir(m.townRoot, dbName)
			fmt.Printf("  Setting up DoltHub remote for %s/%s...\n", org, doltserver.DoltHubRepoName(dbName))
			if err := doltserver.SetupDoltHubRemote(dbDir, org, dbName, token); err != nil {
				fmt.Printf("  Warning: DoltHub remote setup failed: %v\n", err)
				fmt.Printf("  You can set up the remote manually later with 'gt dolt sync'.\n")
			} else {
				fmt.Printf("   ✓ DoltHub remote configured and initial push complete\n")
			}
		}
	}

	// Provision PRIME.md with Gas Town context for all workers in this rig.
	// This is the fallback if SessionStart hook fails - ensures ALL workers
	// (crew, polecats, refinery, witness) have GUPP and essential Gas Town context.
	// PRIME.md is read by bd prime and output to the agent.
	// Use ResolveBeadsDir to follow redirect (writes to mayor/rig/.beads/ if tracked).
	resolvedBeadsPath := beads.ResolveBeadsDir(rigPath)
	if err := beads.ProvisionPrimeMD(resolvedBeadsPath); err != nil {
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
	refineryGit := git.NewGit(refineryRigPath)
	if err := refineryGit.ConfigureHooksPath(); err != nil {
		return nil, fmt.Errorf("configuring hooks for refinery: %w", err)
	}
	fmt.Printf("   ✓ Created refinery worktree\n")
	// Set up beads redirect for refinery (points to rig-level .beads)
	if err := beads.SetupRedirect(m.townRoot, refineryRigPath); err != nil {
		fmt.Printf("  Warning: Could not set up refinery beads redirect: %v\n", err)
	}
	// Copy overlay files from .runtime/overlay/ to refinery root.
	// This allows services to have .env and other config files at their root.
	if err := CopyOverlay(rigPath, refineryRigPath); err != nil {
		// Non-fatal - log warning but continue
		fmt.Printf("  Warning: Could not copy overlay files to refinery: %v\n", err)
	}

	// NOTE: Claude settings are installed by the agent at startup, not here.
	// Claude Code does NOT traverse parent directories for settings.json.
	// See: https://github.com/anthropics/claude-code/issues/12962

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
	// NOTE: Witness hooks are installed by witness/manager.go:Start() via EnsureSettingsForRole.
	// No need to create patrol hooks here — agents self-install at startup.

	// Create polecats directory with .claude/ settings scaffold.
	// Settings are passed to Claude Code via --settings flag at session start.
	// Scaffolding them here ensures the settings file exists before the first
	// polecat session starts, preventing startup failures from missing hooks.
	polecatsPath := filepath.Join(rigPath, "polecats")
	if err := os.MkdirAll(polecatsPath, 0755); err != nil {
		return nil, fmt.Errorf("creating polecats dir: %w", err)
	}
	if err := claude.EnsureSettingsForRole(polecatsPath, "polecat"); err != nil {
		// Non-fatal: session startup will retry via EnsureSettingsForRole
		fmt.Printf("  %s Could not scaffold polecat settings: %v\n", "!", err)
	}
	if err := commands.ProvisionFor(polecatsPath, "claude"); err != nil {
		// Non-fatal: commands are convenience, not critical
		fmt.Printf("  %s Could not scaffold polecat commands: %v\n", "!", err)
	}

	// Register route in town-level routes.jsonl BEFORE creating agent beads.
	// initAgentBeads calls ResolveRoutingTarget which needs the route to exist.
	// Without this, agent bead creation logs "no route found" warnings (#1424).
	if opts.BeadsPrefix != "" {
		routePath := opts.Name
		mayorRigBeads := filepath.Join(rigPath, "mayor", "rig", ".beads")
		if _, err := os.Stat(mayorRigBeads); err == nil {
			routePath = opts.Name + "/mayor/rig"
		}
		route := beads.Route{
			Prefix: opts.BeadsPrefix + "-",
			Path:   routePath,
		}
		if err := beads.AppendRoute(m.townRoot, route); err != nil {
			fmt.Printf("  Warning: Could not update routes.jsonl: %v\n", err)
		}
	}

	// Create rig-level settings directory (used by gt config for rig overrides)
	rigSettingsPath := filepath.Join(rigPath, constants.DirSettings)
	if err := os.MkdirAll(rigSettingsPath, 0755); err != nil {
		return nil, fmt.Errorf("creating settings dir: %w", err)
	}

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

// InitBeads initializes the beads database at rig level.
// The project's .beads/config.yaml determines sync-branch settings.
// Use `bd doctor --fix` in the project to configure sync-branch if needed.
// TODO(bd-yaml): beads config should migrate to JSON (see beads issue)
func (m *Manager) InitBeads(rigPath, prefix string) error {
	// Validate prefix format to prevent command injection from config files
	if !isValidBeadsPrefix(prefix) {
		return fmt.Errorf("invalid beads prefix %q: must be alphanumeric with optional hyphens, start with letter, max 20 chars", prefix)
	}

	beadsDir := filepath.Join(rigPath, ".beads")
	mayorRigBeads := filepath.Join(rigPath, "mayor", "rig", ".beads")

	// Check if source repo has tracked .beads/ (cloned into mayor/rig).
	// If so, create a redirect file instead of a new database.
	if _, err := os.Stat(mayorRigBeads); err == nil {
		// Tracked beads exist - create redirect to mayor/rig/.beads
		if err := os.MkdirAll(beadsDir, 0755); err != nil {
			return err
		}
		redirectPath := filepath.Join(beadsDir, "redirect")
		if err := os.WriteFile(redirectPath, []byte("mayor/rig/.beads\n"), 0644); err != nil {
			return fmt.Errorf("creating redirect file: %w", err)
		}
		return nil
	}

	// No tracked beads - create local database
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

	// Run bd init if available (Dolt is the only backend since bd v0.51.0).
	// --server tells bd to set dolt_mode=server in metadata.json so bd
	// connects to the centralized Dolt sql-server instead of embedded mode.
	cmd := exec.Command("bd", "init", "--prefix", prefix, "--server")
	cmd.Dir = rigPath
	cmd.Env = filteredEnv
	_, bdInitErr := cmd.CombinedOutput()
	if bdInitErr != nil {
		// bd might not be installed or failed, create minimal structure
		// Note: beads currently expects YAML format for config
		configPath := filepath.Join(beadsDir, "config.yaml")
		configContent := fmt.Sprintf("prefix: %s\nissue-prefix: %s\n", prefix, prefix)
		if writeErr := os.WriteFile(configPath, []byte(configContent), 0644); writeErr != nil {
			return writeErr
		}
	} else {
		// bd init succeeded - configure the Dolt database

		// Configure custom types for Gas Town (agent, role, rig, convoy).
		// These were extracted from beads core in v0.46.0 and now require explicit config.
		configCmd := exec.Command("bd", "config", "set", "types.custom", constants.BeadsCustomTypes)
		configCmd.Dir = rigPath
		configCmd.Env = filteredEnv
		// Ignore errors - older beads versions don't need this
		_, _ = configCmd.CombinedOutput()

		// Explicitly set issue_prefix config (bd init --prefix may not persist it in newer versions).
		// Without this, bd create and gt sling fail with "issue_prefix config is missing".
		prefixSetCmd := exec.Command("bd", "config", "set", "issue_prefix", prefix)
		prefixSetCmd.Dir = rigPath
		prefixSetCmd.Env = filteredEnv
		if prefixOutput, prefixErr := prefixSetCmd.CombinedOutput(); prefixErr != nil {
			return fmt.Errorf("bd config set issue_prefix failed: %s", strings.TrimSpace(string(prefixOutput)))
		}
	}

	// Ensure database has repository fingerprint (GH #25).
	// This is idempotent - safe on both new and legacy (pre-0.17.5) databases.
	// Without fingerprint, the bd daemon fails to start silently.
	migrateCmd := exec.Command("bd", "migrate", "--update-repo-id")
	migrateCmd.Dir = rigPath
	migrateCmd.Env = filteredEnv
	// Ignore errors - fingerprint is optional for functionality
	_, _ = migrateCmd.CombinedOutput()

	// Ensure issues.jsonl exists to prevent bd auto-export from corrupting other files.
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

// isStandardBeadHash checks if a string looks like a standard 5-char bead hash.
// Regular bead IDs use a 5-character base32-encoded hash (e.g., "mawit", "z0ixd").
// This distinguishes regular issues from agent beads (suffix like "witness")
// and merge requests (10-char suffix).
func isStandardBeadHash(s string) bool {
	if len(s) != 5 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// bdDatabaseExists checks if a beads directory has an initialized database
// that is actually usable (not just tracked metadata from another workspace).
//
// For Dolt server mode, metadata.json may be tracked in git with dolt_database
// pointing to a database that doesn't exist on this Dolt server. In that case,
// we need to run bd init to create the server-side database.
func bdDatabaseExists(beadsDir string) bool {
	metadataPath := filepath.Join(beadsDir, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return false
	}

	// Parse metadata to check if the referenced Dolt database actually exists.
	var meta struct {
		DoltMode     string `json:"dolt_mode"`
		DoltDatabase string `json:"dolt_database"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return true // Can't parse — assume it exists (backward compat)
	}

	// For server mode, verify the database exists in .dolt-data/.
	// metadata.json may be tracked in git from another workspace where
	// the Dolt server had this database, but this is a fresh server.
	if meta.DoltMode == "server" && meta.DoltDatabase != "" {
		// Walk up from beadsDir to find the town root (.dolt-data lives there).
		townRoot := beads.FindTownRoot(filepath.Dir(beadsDir))
		if townRoot == "" {
			return true // Can't find town root — assume it exists
		}
		dbDir := filepath.Join(townRoot, ".dolt-data", meta.DoltDatabase)
		if _, err := os.Stat(dbDir); os.IsNotExist(err) {
			return false // Database doesn't exist on this server
		}
	}

	return true
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
					return strings.TrimSuffix(value, "-")
				}
			}
		}
	}

	// Fallback: try to detect prefix from existing issues in issues.jsonl
	// Parse multiple lines and only consider regular issue IDs (5-char hashes)
	// to avoid misdetecting agent beads like "gt-demo-witness" as prefix "gt-demo".
	beadsDir := filepath.Dir(configPath)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	if issuesData, err := os.ReadFile(issuesPath); err == nil {
		issuesLines := strings.Split(string(issuesData), "\n")
		var detectedPrefix string
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
					// Only consider IDs with a 5-char alphanumeric hash suffix
					// (standard bead format). This filters out agent beads
					// (gt-demo-witness), merge requests (gt-mr-abc1234567),
					// and other multi-hyphen IDs.
					if dashIdx := strings.LastIndex(issueID, "-"); dashIdx > 0 {
						hash := issueID[dashIdx+1:]
						if !isStandardBeadHash(hash) {
							continue
						}
						prefix := issueID[:dashIdx]
						if !isValidBeadsPrefix(prefix) {
							continue
						}
						if detectedPrefix == "" {
							detectedPrefix = prefix
						} else if detectedPrefix != prefix {
							// Conflicting prefixes — can't determine reliably
							return ""
						}
					}
				}
			}
		}
		if detectedPrefix != "" {
			return detectedPrefix
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
// RegisterRigOptions contains options for registering an existing rig directory.
type RegisterRigOptions struct {
	Name        string // Rig name (directory name)
	GitURL      string // Override git URL (auto-detected from origin if empty)
	BeadsPrefix string // Beads issue prefix (defaults to derived from name or existing config)
	Force       bool   // Register even if directory structure looks incomplete
}

// RegisterRigResult contains the result of registering a rig.
type RegisterRigResult struct {
	Name          string // Rig name
	GitURL        string // Detected or provided git URL
	BeadsPrefix   string // Detected or derived beads prefix
	FromConfig    bool   // True if values were read from existing config.json
	DefaultBranch string // Default branch from existing config (if any)
}

// RegisterRig registers an existing rig directory with the town.
// Complementary to AddRig: while AddRig creates a new rig from scratch,
// RegisterRig adopts an existing directory structure.
func (m *Manager) RegisterRig(opts RegisterRigOptions) (*RegisterRigResult, error) {
	if m.RigExists(opts.Name) {
		return nil, ErrRigExists
	}

	if strings.ContainsAny(opts.Name, "-. ") {
		sanitized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(opts.Name)
		sanitized = strings.ToLower(sanitized)
		return nil, fmt.Errorf("rig name %q contains invalid characters; hyphens, dots, and spaces are reserved for agent ID parsing. Try %q instead (underscores are allowed)", opts.Name, sanitized)
	}

	for _, reserved := range reservedRigNames {
		if strings.EqualFold(opts.Name, reserved) {
			return nil, fmt.Errorf("rig name %q is reserved for town-level infrastructure", opts.Name)
		}
	}

	rigPath := filepath.Join(m.townRoot, opts.Name)

	info, err := os.Stat(rigPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("directory does not exist: %s", rigPath)
	}
	if err != nil {
		return nil, fmt.Errorf("checking directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", rigPath)
	}

	result := &RegisterRigResult{Name: opts.Name}

	// Try to load existing config.json
	existingConfig, err := LoadRigConfig(rigPath)
	if err == nil && existingConfig != nil {
		result.FromConfig = true
		if opts.GitURL == "" {
			result.GitURL = existingConfig.GitURL
		}
		if opts.BeadsPrefix == "" && existingConfig.Beads != nil {
			result.BeadsPrefix = existingConfig.Beads.Prefix
		}
		result.DefaultBranch = existingConfig.DefaultBranch
	}

	// If no git URL, try to detect from git remote
	if result.GitURL == "" && opts.GitURL == "" {
		detectedURL, detectErr := m.detectGitURL(rigPath)
		if detectErr != nil && !opts.Force {
			return nil, fmt.Errorf("could not detect git URL (use --url to specify, or --force to skip): %w", detectErr)
		}
		result.GitURL = detectedURL
	}
	if opts.GitURL != "" {
		result.GitURL = opts.GitURL
	}

	// Derive beads prefix
	if result.BeadsPrefix == "" && opts.BeadsPrefix == "" {
		result.BeadsPrefix = deriveBeadsPrefix(opts.Name)
	}
	if opts.BeadsPrefix != "" {
		result.BeadsPrefix = opts.BeadsPrefix
	}

	// Register in town config
	m.config.Rigs[opts.Name] = config.RigEntry{
		GitURL:  result.GitURL,
		AddedAt: time.Now(),
		BeadsConfig: &config.BeadsConfig{
			Prefix: result.BeadsPrefix,
		},
	}

	return result, nil
}

// detectGitURL attempts to detect the git remote URL from an existing repository.
func (m *Manager) detectGitURL(rigPath string) (string, error) {
	possiblePaths := []string{
		rigPath,
		filepath.Join(rigPath, "mayor", "rig"),
		filepath.Join(rigPath, "refinery", "rig"),
	}
	for _, p := range possiblePaths {
		g := git.NewGitWithDir(p, "")
		url, err := g.RemoteURL("origin")
		if err == nil && url != "" {
			return strings.TrimSpace(url), nil
		}
	}
	return "", fmt.Errorf("no git repository with origin remote found in %s", rigPath)
}

func (m *Manager) ListRigNames() []string {
	names := make([]string, 0, len(m.config.Rigs))
	for name := range m.config.Rigs {
		names = append(names, name)
	}
	return names
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

	// Add plugins/, .repo.git/, and .land-worktree/ to rig .gitignore
	gitignorePath := filepath.Join(rigPath, ".gitignore")
	if err := m.ensureGitignoreEntry(gitignorePath, "plugins/"); err != nil {
		return err
	}
	if err := m.ensureGitignoreEntry(gitignorePath, ".repo.git/"); err != nil {
		return err
	}
	return m.ensureGitignoreEntry(gitignorePath, ".land-worktree/")
}

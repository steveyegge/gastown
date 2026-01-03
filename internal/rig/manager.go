package rig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/templates"
)

// Common errors
var (
	ErrRigNotFound = errors.New("rig not found")
	ErrRigExists   = errors.New("rig already exists")
)

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
func (m *Manager) DiscoverRigs() ([]*Rig, error) {
	var rigs []*Rig

	for name, entry := range m.config.Rigs {
		rig, err := m.loadRig(name, entry)
		if err != nil {
			// Log error but continue with other rigs
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
			if e.IsDir() {
				rig.Polecats = append(rig.Polecats, e.Name())
			}
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

	// Check for witness (witnesses don't have clones, just state.json)
	witnessStatePath := filepath.Join(rigPath, "witness", "state.json")
	if _, err := os.Stat(witnessStatePath); err == nil {
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
	Name        string // Rig name (directory name)
	GitURL      string // Repository URL
	BeadsPrefix string // Beads issue prefix (defaults to derived from name)
	LocalRepo   string // Optional local repo for reference clones
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
		sanitized := strings.NewReplacer("-", "", ".", "", " ", "").Replace(opts.Name)
		return nil, fmt.Errorf("rig name %q contains invalid characters (hyphens, dots, or spaces break agent ID parsing); use %q instead", opts.Name, sanitized)
	}

	rigPath := filepath.Join(m.townRoot, opts.Name)

	// Check if directory already exists
	if _, err := os.Stat(rigPath); err == nil {
		return nil, fmt.Errorf("directory already exists: %s", rigPath)
	}

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
	bareRepoPath := filepath.Join(rigPath, ".repo.git")
	if localRepo != "" {
		if err := m.git.CloneBareWithReference(opts.GitURL, bareRepoPath, localRepo); err != nil {
			fmt.Printf("  Warning: could not use local repo reference: %v\n", err)
			_ = os.RemoveAll(bareRepoPath)
			if err := m.git.CloneBare(opts.GitURL, bareRepoPath); err != nil {
				return nil, fmt.Errorf("creating bare repo: %w", err)
			}
		}
	} else {
		if err := m.git.CloneBare(opts.GitURL, bareRepoPath); err != nil {
			return nil, fmt.Errorf("creating bare repo: %w", err)
		}
	}
	bareGit := git.NewGitWithDir(bareRepoPath, "")

	// Detect default branch (main, master, etc.)
	defaultBranch := bareGit.DefaultBranch()
	rigConfig.DefaultBranch = defaultBranch
	// Re-save config with default branch
	if err := m.saveRigConfig(rigPath, rigConfig); err != nil {
		return nil, fmt.Errorf("updating rig config with default branch: %w", err)
	}

	// Create mayor as regular clone (separate from bare repo).
	// Mayor doesn't need to see polecat branches - that's refinery's job.
	// This also allows mayor to stay on the default branch without conflicting with refinery.
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

	// Check if source repo has .beads/ with its own prefix - if so, use that prefix.
	// This ensures we use the project's existing beads database instead of creating a new one.
	// Without this, routing would fail when trying to access existing issues because the
	// rig config would have a different prefix than what the issues actually use.
	sourceBeadsConfig := filepath.Join(mayorRigPath, ".beads", "config.yaml")
	if _, err := os.Stat(sourceBeadsConfig); err == nil {
		if sourcePrefix := detectBeadsPrefixFromConfig(sourceBeadsConfig); sourcePrefix != "" {
			fmt.Printf("  Detected existing beads prefix '%s' from source repo\n", sourcePrefix)
			opts.BeadsPrefix = sourcePrefix
			rigConfig.Beads.Prefix = sourcePrefix
			// Re-save rig config with detected prefix
			if err := m.saveRigConfig(rigPath, rigConfig); err != nil {
				return nil, fmt.Errorf("updating rig config with detected prefix: %w", err)
			}
			// Initialize bd database with the detected prefix.
			// beads.db is gitignored so it doesn't exist after clone - we need to create it.
			// bd init --prefix will create the database and auto-import from issues.jsonl.
			sourceBeadsDB := filepath.Join(mayorRigPath, ".beads", "beads.db")
			if _, err := os.Stat(sourceBeadsDB); os.IsNotExist(err) {
				cmd := exec.Command("bd", "init", "--prefix", sourcePrefix)
				cmd.Dir = mayorRigPath
				if output, err := cmd.CombinedOutput(); err != nil {
					fmt.Printf("  Warning: Could not init bd database: %v (%s)\n", err, strings.TrimSpace(string(output)))
				}
			}
		}
	}

	// Create mayor CLAUDE.md (overrides any from cloned repo)
	if err := m.createRoleCLAUDEmd(mayorRigPath, "mayor", opts.Name, ""); err != nil {
		return nil, fmt.Errorf("creating mayor CLAUDE.md: %w", err)
	}

	// Create refinery as worktree from bare repo on default branch.
	// Refinery needs to see polecat branches (shared .repo.git) and merges them.
	// Being on the default branch allows direct merge workflow.
	refineryRigPath := filepath.Join(rigPath, "refinery", "rig")
	if err := os.MkdirAll(filepath.Dir(refineryRigPath), 0755); err != nil {
		return nil, fmt.Errorf("creating refinery dir: %w", err)
	}
	if err := bareGit.WorktreeAddExisting(refineryRigPath, defaultBranch); err != nil {
		return nil, fmt.Errorf("creating refinery worktree: %w", err)
	}
	// Create refinery CLAUDE.md (overrides any from cloned repo)
	if err := m.createRoleCLAUDEmd(refineryRigPath, "refinery", opts.Name, ""); err != nil {
		return nil, fmt.Errorf("creating refinery CLAUDE.md: %w", err)
	}
	// Create refinery hooks for patrol triggering (at refinery/ level, not rig/)
	refineryPath := filepath.Dir(refineryRigPath)
	if err := m.createPatrolHooks(refineryPath); err != nil {
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
	if err := m.createPatrolHooks(witnessPath); err != nil {
		fmt.Printf("  Warning: Could not create witness hooks: %v\n", err)
	}

	// Create polecats directory (empty)
	polecatsPath := filepath.Join(rigPath, "polecats")
	if err := os.MkdirAll(polecatsPath, 0755); err != nil {
		return nil, fmt.Errorf("creating polecats dir: %w", err)
	}

	// Initialize agent state files
	if err := m.initAgentStates(rigPath); err != nil {
		return nil, fmt.Errorf("initializing agent states: %w", err)
	}

	// Initialize beads at rig level
	if err := m.initBeads(rigPath, opts.BeadsPrefix); err != nil {
		return nil, fmt.Errorf("initializing beads: %w", err)
	}

	// Create agent beads for this rig (witness, refinery) and
	// global agents (deacon, mayor) if this is the first rig.
	isFirstRig := len(m.config.Rigs) == 0
	if err := m.initAgentBeads(rigPath, opts.Name, opts.BeadsPrefix, isFirstRig); err != nil {
		// Non-fatal: log warning but continue
		fmt.Printf("  Warning: Could not create agent beads: %v\n", err)
	}

	// Seed patrol molecules for this rig
	if err := m.seedPatrolMolecules(rigPath); err != nil {
		// Non-fatal: log warning but continue
		fmt.Printf("  Warning: Could not seed patrol molecules: %v\n", err)
	}

	// Create plugin directories
	if err := m.createPluginDirectories(rigPath); err != nil {
		// Non-fatal: log warning but continue
		fmt.Printf("  Warning: Could not create plugin directories: %v\n", err)
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

// initAgentStates creates initial state.json files for agents.
func (m *Manager) initAgentStates(rigPath string) error {
	agents := []struct {
		path string
		role string
	}{
		{filepath.Join(rigPath, "refinery", "state.json"), "refinery"},
		{filepath.Join(rigPath, "witness", "state.json"), "witness"},
		{filepath.Join(rigPath, "mayor", "state.json"), "mayor"},
	}

	for _, agent := range agents {
		state := &config.AgentState{
			Role:       agent.role,
			LastActive: time.Now(),
		}
		data, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(agent.path, data, 0644); err != nil {
			return err
		}
	}
	return nil
}

// initBeads initializes the beads database at rig level.
// The project's .beads/config.yaml determines sync-branch settings.
// Use `bd doctor --fix` in the project to configure sync-branch if needed.
// TODO(bd-yaml): beads config should migrate to JSON (see beads issue)
func (m *Manager) initBeads(rigPath, prefix string) error {
	beadsDir := filepath.Join(rigPath, ".beads")
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

	// Run bd init if available
	cmd := exec.Command("bd", "init", "--prefix", prefix)
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

	// Ensure database has repository fingerprint (GH #25).
	// This is idempotent - safe on both new and legacy (pre-0.17.5) databases.
	// Without fingerprint, the bd daemon fails to start silently.
	migrateCmd := exec.Command("bd", "migrate", "--update-repo-id")
	migrateCmd.Dir = rigPath
	migrateCmd.Env = filteredEnv
	// Ignore errors - fingerprint is optional for functionality
	_, _ = migrateCmd.CombinedOutput()

	return nil
}

// initAgentBeads creates agent beads for this rig and optionally global agents.
// - Always creates: gt-<rig>-witness, gt-<rig>-refinery
// - First rig only: gt-deacon, gt-mayor
//
// Agent beads are stored in the TOWN beads (not rig beads) because they use
// the canonical gt-* prefix for cross-rig coordination. The town beads must
// be initialized with 'gt' prefix for this to work.
//
// Agent beads track lifecycle state for ZFC compliance (gt-h3hak, gt-pinkq).
func (m *Manager) initAgentBeads(rigPath, rigName, prefix string, isFirstRig bool) error {
	// Agent beads go in town beads (gt-* prefix), not rig beads.
	// This enables cross-rig agent coordination via canonical IDs.
	townBeadsDir := filepath.Join(m.townRoot, ".beads")
	bd := beads.NewWithBeadsDir(m.townRoot, townBeadsDir)

	// Define agents to create
	type agentDef struct {
		id       string
		roleType string
		rig      string
		desc     string
	}

	var agents []agentDef

	// Always create rig-specific agents using canonical gt- prefix.
	// Agent bead IDs use the gastown namespace (gt-) regardless of the rig's
	// beads prefix. Format: gt-<rig>-<role> (e.g., gt-tribal-witness)
	agents = append(agents,
		agentDef{
			id:       beads.WitnessBeadID(rigName),
			roleType: "witness",
			rig:      rigName,
			desc:     fmt.Sprintf("Witness for %s - monitors polecat health and progress.", rigName),
		},
		agentDef{
			id:       beads.RefineryBeadID(rigName),
			roleType: "refinery",
			rig:      rigName,
			desc:     fmt.Sprintf("Refinery for %s - processes merge queue.", rigName),
		},
	)

	// First rig also gets global agents (deacon, mayor)
	if isFirstRig {
		agents = append(agents,
			agentDef{
				id:       beads.DeaconBeadID(),
				roleType: "deacon",
				rig:      "",
				desc:     "Deacon (daemon beacon) - receives mechanical heartbeats, runs town plugins and monitoring.",
			},
			agentDef{
				id:       beads.MayorBeadID(),
				roleType: "mayor",
				rig:      "",
				desc:     "Mayor - global coordinator, handles cross-rig communication and escalations.",
			},
		)
	}

	for _, agent := range agents {
		// Check if already exists
		if _, err := bd.Show(agent.id); err == nil {
			continue // Already exists
		}

		// RoleBead points to the shared role definition bead for this agent type.
		// Role beads are shared: gt-witness-role, gt-refinery-role, etc.
		fields := &beads.AgentFields{
			RoleType:   agent.roleType,
			Rig:        agent.rig,
			AgentState: "idle",
			HookBead:   "",
			RoleBead:   "gt-" + agent.roleType + "-role",
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
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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

// detectBeadsPrefixFromConfig reads the issue prefix from a beads config.yaml file.
// Returns empty string if the file doesn't exist or doesn't contain a prefix.
// Falls back to detecting prefix from existing issues in issues.jsonl.
//
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
				if value != "" {
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
						return prefix
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

// createRoleCLAUDEmd creates a CLAUDE.md file with role-specific context.
// This ensures each workspace (crew, refinery, mayor) gets the correct prompting,
// overriding any CLAUDE.md that may exist in the cloned repository.
func (m *Manager) createRoleCLAUDEmd(workspacePath string, role string, rigName string, workerName string) error {
	tmpl, err := templates.New()
	if err != nil {
		return err
	}

	data := templates.RoleData{
		Role:     role,
		RigName:  rigName,
		TownRoot: m.townRoot,
		WorkDir:  workspacePath,
		Polecat:  workerName, // Used for crew member name as well
	}

	content, err := tmpl.RenderRole(role, data)
	if err != nil {
		return err
	}

	claudePath := filepath.Join(workspacePath, "CLAUDE.md")
	return os.WriteFile(claudePath, []byte(content), 0644)
}

// createPatrolHooks creates .claude/settings.json with hooks for patrol roles.
// These hooks trigger gt prime on session start and inject mail, enabling
// autonomous patrol execution for Witness and Refinery roles.
func (m *Manager) createPatrolHooks(workspacePath string) error {
	claudeDir := filepath.Join(workspacePath, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
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
	settingsPath := filepath.Join(claudeDir, "settings.json")
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
		cmd := exec.Command("bd", "create",
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

	// Add plugins/ to rig .gitignore
	gitignorePath := filepath.Join(rigPath, ".gitignore")
	return m.ensureGitignoreEntry(gitignorePath, "plugins/")
}

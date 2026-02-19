// Package tmuxinator provides declarative YAML-based tmux session configuration
// using tmuxinator. It generates tmuxinator config files from Gas Town's
// SessionConfig and wraps the tmuxinator CLI for session lifecycle management.
//
// Runtime tmux operations (send-keys, nudges, kill, health checks, capture)
// remain as raw tmux calls since tmuxinator only handles session creation.
package tmuxinator

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/tmux"
	"gopkg.in/yaml.v3"
)

// Config represents a tmuxinator YAML configuration file.
// Fields map directly to tmuxinator's expected YAML structure.
type Config struct {
	Name            string     `yaml:"name"`
	Root            string     `yaml:"root"`
	Attach          bool       `yaml:"attach"`
	OnProjectStart  []string   `yaml:"on_project_start,omitempty"`
	Windows         []Window   `yaml:"windows"`
}

// Window represents a tmuxinator window configuration.
type Window struct {
	Name  string
	Panes []string
}

// windowEntry is used for YAML marshaling of windows in tmuxinator format.
type windowEntry map[string]windowPanes

type windowPanes struct {
	Panes []string `yaml:"panes"`
}

// MarshalYAML implements custom YAML marshaling for Window to produce
// the tmuxinator-expected format:
//
//	- agent:
//	    panes:
//	      - exec env ...
func (w Window) MarshalYAML() (interface{}, error) {
	return windowEntry{
		w.Name: windowPanes{Panes: w.Panes},
	}, nil
}

// SessionConfig mirrors session.SessionConfig fields needed for config generation.
// This avoids a circular import between tmuxinator and session packages.
type SessionConfig struct {
	SessionID        string
	WorkDir          string
	Role             string
	TownRoot         string
	RigPath          string
	RigName          string
	AgentName        string
	Command          string
	Theme            *tmux.Theme
	ExtraEnv         map[string]string
	RemainOnExit     bool
	AutoRespawn      bool
	RuntimeConfigDir string
}

// FromSessionConfig creates a tmuxinator Config from a SessionConfig.
// It generates:
//   - on_project_start hooks for: remain-on-exit, env vars, theme, bindings, mouse mode
//   - window pane commands from the startup command
func FromSessionConfig(cfg SessionConfig) (*Config, error) {
	if cfg.SessionID == "" {
		return nil, fmt.Errorf("SessionID is required")
	}
	if cfg.WorkDir == "" {
		return nil, fmt.Errorf("WorkDir is required")
	}
	if cfg.Role == "" {
		return nil, fmt.Errorf("Role is required")
	}

	c := &Config{
		Name:   cfg.SessionID,
		Root:   cfg.WorkDir,
		Attach: false,
	}

	var hooks []string

	// 1. Remain-on-exit (must be first so pane survives early crashes)
	if cfg.RemainOnExit {
		hooks = append(hooks, fmt.Sprintf("tmux set-option -t %s remain-on-exit on", cfg.SessionID))
	}

	// 2. Environment variables
	envVars := config.AgentEnv(config.AgentEnvConfig{
		Role:             cfg.Role,
		Rig:              cfg.RigName,
		AgentName:        cfg.AgentName,
		TownRoot:         cfg.TownRoot,
		RuntimeConfigDir: cfg.RuntimeConfigDir,
	})
	// Merge extra env vars
	for k, v := range cfg.ExtraEnv {
		envVars[k] = v
	}
	// Sort keys for deterministic output
	envKeys := make([]string, 0, len(envVars))
	for k := range envVars {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	for _, k := range envKeys {
		hooks = append(hooks, fmt.Sprintf("tmux set-environment -t %s %s %s",
			cfg.SessionID, k, shellQuote(envVars[k])))
	}

	// 3. Theme and status bar
	if cfg.Theme != nil {
		hooks = append(hooks, generateThemeHooks(cfg.SessionID, *cfg.Theme, cfg.RigName, cfg.AgentName, cfg.Role)...)
	}

	// 4. Mouse and clipboard
	if cfg.Theme != nil {
		hooks = append(hooks,
			fmt.Sprintf("tmux set-option -t %s mouse on", cfg.SessionID),
			fmt.Sprintf("tmux set-option -t %s set-clipboard on", cfg.SessionID),
		)
	}

	// 5. Key bindings (only if themed — unthemed sessions are utility sessions)
	if cfg.Theme != nil {
		hooks = append(hooks, generateBindingHooks(cfg.SessionID)...)
	}

	// 6. Auto-respawn hook
	if cfg.AutoRespawn {
		safeSession := strings.ReplaceAll(cfg.SessionID, "'", "'\\''")
		hooks = append(hooks,
			fmt.Sprintf(`tmux set-hook -t %s pane-died "run-shell \"sleep 3 && tmux respawn-pane -k -t '%s' && tmux set-option -t '%s' remain-on-exit on\""`,
				cfg.SessionID, safeSession, safeSession),
		)
	}

	c.OnProjectStart = hooks

	// Window with agent command
	paneCmd := cfg.Command
	if paneCmd == "" {
		paneCmd = "bash" // fallback
	}
	c.Windows = []Window{
		{
			Name:  "agent",
			Panes: []string{paneCmd},
		},
	}

	return c, nil
}

// generateThemeHooks produces tmux commands for status bar theming.
func generateThemeHooks(session string, theme tmux.Theme, rig, worker, role string) []string {
	var hooks []string

	// Status style
	hooks = append(hooks, fmt.Sprintf("tmux set-option -t %s status-style %s",
		session, shellQuote(theme.Style())))

	// Status left (icon + identity)
	icon := roleIcon(role)
	var left string
	if rig == "" {
		left = fmt.Sprintf("%s %s ", icon, worker)
	} else {
		left = fmt.Sprintf("%s %s ", icon, session)
	}
	hooks = append(hooks,
		fmt.Sprintf("tmux set-option -t %s status-left-length 25", session),
		fmt.Sprintf("tmux set-option -t %s status-left %s", session, shellQuote(left)),
	)

	// Status right (dynamic)
	right := fmt.Sprintf(`#(gt status-line --session=%s 2>/dev/null) %%H:%%M`, session)
	hooks = append(hooks,
		fmt.Sprintf("tmux set-option -t %s status-right-length 80", session),
		fmt.Sprintf("tmux set-option -t %s status-interval 5", session),
		fmt.Sprintf("tmux set-option -t %s status-right %s", session, shellQuote(right)),
	)

	return hooks
}

// generateBindingHooks produces tmux key binding commands.
func generateBindingHooks(session string) []string {
	pattern := sessionPrefixPattern()
	ifShell := fmt.Sprintf("echo '#{session_name}' | grep -Eq '%s'", pattern)

	return []string{
		// Mail click
		fmt.Sprintf(`tmux bind-key -T root MouseDown1StatusRight display-popup -E -w 60 -h 15 "gt mail peek || echo 'No unread mail'"`),
		// Cycle bindings
		fmt.Sprintf(`tmux bind-key -T prefix n if-shell "%s" "run-shell 'gt cycle next --session #{session_name}'" next-window`, ifShell),
		fmt.Sprintf(`tmux bind-key -T prefix p if-shell "%s" "run-shell 'gt cycle prev --session #{session_name}'" previous-window`, ifShell),
		// Feed binding
		fmt.Sprintf(`tmux bind-key -T prefix a if-shell "%s" "run-shell 'gt feed --window'" "display-message 'C-b a is for Gas Town sessions only'"`, ifShell),
		// Agents binding
		fmt.Sprintf(`tmux bind-key -T prefix g if-shell "%s" "run-shell 'gt agents'" "display-message 'C-b g is for Gas Town sessions only'"`, ifShell),
	}
}

// sessionPrefixPattern returns a grep -Eq pattern matching Gas Town session names.
// Mirrors tmux.sessionPrefixPattern() to generate identical patterns.
func sessionPrefixPattern() string {
	seen := map[string]bool{"hq": true, "gt": true}
	townRoot := os.Getenv("GT_ROOT")
	if townRoot != "" {
		for _, p := range config.AllRigPrefixes(townRoot) {
			if isValidPrefix(p) {
				seen[p] = true
			}
		}
	}
	sorted := make([]string, 0, len(seen))
	for p := range seen {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	return "^(" + strings.Join(sorted, "|") + ")-"
}

// isValidPrefix validates a prefix for safe use in grep patterns.
func isValidPrefix(p string) bool {
	if len(p) == 0 || len(p) > 20 {
		return false
	}
	if p[0] < 'a' || p[0] > 'z' {
		if p[0] < 'A' || p[0] > 'Z' {
			return false
		}
	}
	for _, c := range p[1:] {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// roleIcon returns the emoji icon for a role.
func roleIcon(role string) string {
	switch role {
	case constants.RoleMayor, "coordinator":
		return constants.EmojiMayor
	case constants.RoleDeacon, "health-check":
		return constants.EmojiDeacon
	case constants.RoleWitness:
		return constants.EmojiWitness
	case constants.RoleRefinery:
		return constants.EmojiRefinery
	case constants.RoleCrew:
		return constants.EmojiCrew
	case constants.RolePolecat:
		return constants.EmojiPolecat
	default:
		return ""
	}
}

// shellQuote wraps a value in single quotes for safe use in shell commands.
// Empty values are represented as '' to preserve the assignment.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// If no special chars, return as-is
	needsQuoting := false
	for _, c := range s {
		switch c {
		case ' ', '\t', '\n', '"', '\'', '`', '$', '\\', '!', '*', '?',
			'[', ']', '{', '}', '(', ')', '<', '>', '|', '&', ';', '#',
			'%', ',', '=':
			needsQuoting = true
		}
		if needsQuoting {
			break
		}
	}
	if !needsQuoting {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// ToYAML marshals the Config to YAML bytes.
func (c *Config) ToYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

// WriteToFile marshals the Config to YAML and writes it to the specified path.
func (c *Config) WriteToFile(path string) error {
	data, err := c.ToYAML()
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	return nil
}

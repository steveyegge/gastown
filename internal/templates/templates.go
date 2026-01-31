// Package templates provides embedded templates for role contexts and messages.
package templates

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed roles/*.md.tmpl messages/*.md.tmpl
var templateFS embed.FS

//go:embed commands/*.md
var commandsFS embed.FS

// Templates manages role and message templates.
type Templates struct {
	roleTemplates    *template.Template
	messageTemplates *template.Template
}

// RoleData contains information for rendering role contexts.
type RoleData struct {
	Role           string   // mayor, witness, refinery, polecat, crew, deacon
	RigName        string   // e.g., "greenplace"
	TownRoot       string   // e.g., "/Users/steve/ai"
	TownName       string   // e.g., "ai" - the town identifier for session names
	WorkDir        string   // current working directory
	DefaultBranch  string   // default branch for merges (e.g., "main", "develop")
	Polecat        string   // polecat name (for polecat role)
	Polecats       []string // list of polecats (for witness role)
	BeadsDir       string   // BEADS_DIR path
	IssuePrefix    string   // beads issue prefix
	MayorSession   string   // e.g., "gt-ai-mayor" - dynamic mayor session name
	DeaconSession  string   // e.g., "gt-ai-deacon" - dynamic deacon session name
}

// SpawnData contains information for spawn assignment messages.
type SpawnData struct {
	Issue       string
	Title       string
	Priority    int
	Description string
	Branch      string
	RigName     string
	Polecat     string
}

// NudgeData contains information for nudge messages.
type NudgeData struct {
	Polecat    string
	Reason     string
	NudgeCount int
	MaxNudges  int
	Issue      string
	Status     string
}

// EscalationData contains information for escalation messages.
type EscalationData struct {
	Polecat     string
	Issue       string
	Reason      string
	NudgeCount  int
	LastStatus  string
	Suggestions []string
}

// HandoffData contains information for session handoff messages.
type HandoffData struct {
	Role        string
	CurrentWork string
	Status      string
	NextSteps   []string
	Notes       string
	PendingMail int
	GitBranch   string
	GitDirty    bool
}

// New creates a new Templates instance.
func New() (*Templates, error) {
	t := &Templates{}

	// Parse role templates
	roleTempl, err := template.ParseFS(templateFS, "roles/*.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing role templates: %w", err)
	}
	t.roleTemplates = roleTempl

	// Parse message templates
	msgTempl, err := template.ParseFS(templateFS, "messages/*.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing message templates: %w", err)
	}
	t.messageTemplates = msgTempl

	return t, nil
}

// RenderRole renders a role context template.
func (t *Templates) RenderRole(role string, data RoleData) (string, error) {
	templateName := role + ".md.tmpl"

	var buf bytes.Buffer
	if err := t.roleTemplates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", fmt.Errorf("rendering role template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// RenderMessage renders a message template.
func (t *Templates) RenderMessage(name string, data interface{}) (string, error) {
	templateName := name + ".md.tmpl"

	var buf bytes.Buffer
	if err := t.messageTemplates.ExecuteTemplate(&buf, templateName, data); err != nil {
		return "", fmt.Errorf("rendering message template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// RoleNames returns the list of available role templates.
func (t *Templates) RoleNames() []string {
	return []string{"mayor", "witness", "refinery", "polecat", "crew", "deacon"}
}

// MessageNames returns the list of available message templates.
func (t *Templates) MessageNames() []string {
	return []string{"spawn", "nudge", "escalation", "handoff"}
}

// CreateMayorCLAUDEmd creates the Mayor's CLAUDE.md file at the specified directory.
// This is used by both gt install and gt doctor --fix.
//
// Returns (created bool, error) - created is false if file already exists.
// Existing files are preserved to respect user customizations.
func CreateMayorCLAUDEmd(mayorDir, townRoot, townName, mayorSession, deaconSession string) (bool, error) {
	claudePath := filepath.Join(mayorDir, "CLAUDE.md")

	// Check if file already exists - preserve user customizations
	if _, err := os.Stat(claudePath); err == nil {
		return false, nil // File exists, preserve it
	} else if !os.IsNotExist(err) {
		return false, err // Unexpected error
	}

	tmpl, err := New()
	if err != nil {
		return false, err
	}

	data := RoleData{
		Role:          "mayor",
		TownRoot:      townRoot,
		TownName:      townName,
		WorkDir:       mayorDir,
		MayorSession:  mayorSession,
		DeaconSession: deaconSession,
	}

	content, err := tmpl.RenderRole("mayor", data)
	if err != nil {
		return false, err
	}

	return true, os.WriteFile(claudePath, []byte(content), 0644)
}

// GetAllRoleTemplates returns all role templates as a map of filename to content.
func GetAllRoleTemplates() (map[string][]byte, error) {
	entries, err := templateFS.ReadDir("roles")
	if err != nil {
		return nil, fmt.Errorf("reading roles directory: %w", err)
	}

	result := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := templateFS.ReadFile("roles/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		result[entry.Name()] = content
	}

	return result, nil
}

// ProvisionCommands creates the .claude/commands/ directory with standard slash commands.
// This ensures crew/polecat workspaces have the handoff command and other utilities
// even if the source repo doesn't have them tracked.
// If a command already exists, it is skipped (no overwrite).
func ProvisionCommands(workspacePath string) error {
	entries, err := commandsFS.ReadDir("commands")
	if err != nil {
		return fmt.Errorf("reading commands directory: %w", err)
	}

	// Create .claude/commands/ directory
	commandsDir := filepath.Join(workspacePath, ".claude", "commands")
	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("creating commands directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		destPath := filepath.Join(commandsDir, entry.Name())

		// Skip if command already exists (don't overwrite user customizations)
		if _, err := os.Stat(destPath); err == nil {
			continue
		}

		content, err := commandsFS.ReadFile("commands/" + entry.Name())
		if err != nil {
			return fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil { //nolint:gosec // G306: template files are non-sensitive
			return fmt.Errorf("writing %s: %w", entry.Name(), err)
		}
	}

	return nil
}

// CommandNames returns the list of embedded slash commands.
func CommandNames() ([]string, error) {
	entries, err := commandsFS.ReadDir("commands")
	if err != nil {
		return nil, fmt.Errorf("reading commands directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// HasCommands checks if a workspace has the .claude/commands/ directory provisioned.
func HasCommands(workspacePath string) bool {
	commandsDir := filepath.Join(workspacePath, ".claude", "commands")
	info, err := os.Stat(commandsDir)
	return err == nil && info.IsDir()
}

// MissingCommands returns the list of embedded commands missing from the workspace.
func MissingCommands(workspacePath string) ([]string, error) {
	entries, err := commandsFS.ReadDir("commands")
	if err != nil {
		return nil, fmt.Errorf("reading commands directory: %w", err)
	}

	commandsDir := filepath.Join(workspacePath, ".claude", "commands")
	var missing []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		destPath := filepath.Join(commandsDir, entry.Name())
		if _, err := os.Stat(destPath); os.IsNotExist(err) {
			missing = append(missing, entry.Name())
		}
	}

	return missing, nil
}

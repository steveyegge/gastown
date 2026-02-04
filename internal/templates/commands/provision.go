// Package commands provides agent-agnostic command provisioning.
package commands

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed bodies/*.md
var bodiesFS embed.FS

// Field represents a frontmatter key-value pair.
type Field struct {
	Key   string
	Value string
}

// Command defines a slash command with agent-specific frontmatter.
type Command struct {
	Name        string
	Description string
	AgentFields map[string][]Field
}

// Agent defines an agent's configuration directory.
type Agent struct {
	ConfigDir string // .claude, .opencode, etc.
}

// Agents maps agent names to their configurations.
var Agents = map[string]Agent{
	"claude":   {ConfigDir: ".claude"},
	"opencode": {ConfigDir: ".opencode"},
}

// Commands is the registry of available commands.
var Commands = []Command{
	{
		Name:        "handoff",
		Description: "Hand off to fresh session, work continues from hook",
		AgentFields: map[string][]Field{
			"claude": {
				{"allowed-tools", "Bash(gt handoff:*)"},
				{"argument-hint", "[message]"},
			},
			// opencode: no extra fields, just description
		},
	},
}

// BuildCommand assembles frontmatter + body for an agent.
func BuildCommand(cmd Command, agent string) (string, error) {
	body, err := bodiesFS.ReadFile("bodies/" + cmd.Name + ".md")
	if err != nil {
		return "", fmt.Errorf("reading body: %w", err)
	}

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("description: %s\n", cmd.Description))

	if fields, ok := cmd.AgentFields[agent]; ok {
		for _, f := range fields {
			b.WriteString(fmt.Sprintf("%s: %s\n", f.Key, f.Value))
		}
	}

	b.WriteString("---\n\n")
	b.Write(body)

	return b.String(), nil
}

// ProvisionFor provisions commands for an agent.
func ProvisionFor(workspacePath, agent string) error {
	agent = strings.ToLower(agent)
	cfg, ok := Agents[agent]
	if !ok {
		return fmt.Errorf("unknown agent: %s", agent)
	}

	dir := filepath.Join(workspacePath, cfg.ConfigDir, "commands")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating dir: %w", err)
	}

	for _, cmd := range Commands {
		path := filepath.Join(dir, cmd.Name+".md")

		// Don't overwrite existing
		if _, err := os.Stat(path); err == nil {
			continue
		}

		content, err := BuildCommand(cmd, agent)
		if err != nil {
			return fmt.Errorf("building %s: %w", cmd.Name, err)
		}

		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", cmd.Name, err)
		}
	}

	return nil
}

// MissingFor returns commands missing for an agent.
func MissingFor(workspacePath, agent string) []string {
	agent = strings.ToLower(agent)
	cfg, ok := Agents[agent]
	if !ok {
		return nil
	}

	dir := filepath.Join(workspacePath, cfg.ConfigDir, "commands")
	var missing []string

	for _, cmd := range Commands {
		path := filepath.Join(dir, cmd.Name+".md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, cmd.Name)
		}
	}

	return missing
}

// Names returns the names of all registered commands.
func Names() []string {
	names := make([]string, len(Commands))
	for i, cmd := range Commands {
		names[i] = cmd.Name
	}
	return names
}

// IsKnownAgent returns true if the agent is registered.
func IsKnownAgent(agent string) bool {
	_, ok := Agents[strings.ToLower(agent)]
	return ok
}

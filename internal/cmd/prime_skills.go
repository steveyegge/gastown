package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SkillInfo holds parsed skill metadata from SKILL.md frontmatter
type SkillInfo struct {
	Name        string
	Description string
	Triggers    []string
}

// AgentInfo holds parsed agent metadata from agent .md frontmatter
type AgentInfo struct {
	Name        string
	Description string
	Model       string
}

// DiscoverSkills scans ~/.claude/skills/ for SKILL.md files
func DiscoverSkills() ([]SkillInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	skillsDir := filepath.Join(homeDir, ".claude", "skills")
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil // Graceful degradation
	}

	return scanSkillDirectory(skillsDir)
}

// DiscoverAgents scans ~/.claude/agents/ for agent .md files
func DiscoverAgents() ([]AgentInfo, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	agentsDir := filepath.Join(homeDir, ".claude", "agents")
	if _, err := os.Stat(agentsDir); os.IsNotExist(err) {
		return nil, nil // Graceful degradation
	}

	return scanAgentDirectory(agentsDir)
}

// scanSkillDirectory scans a directory for SKILL.md files (one level deep)
func scanSkillDirectory(dir string) ([]SkillInfo, error) {
	var skills []SkillInfo

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, entry.Name(), "SKILL.md")
		skill, err := parseSkillFile(skillPath)
		if err != nil {
			continue
		}
		if skill != nil {
			skills = append(skills, *skill)
		}
	}

	return skills, nil
}

// scanAgentDirectory scans a directory for agent .md files
func scanAgentDirectory(dir string) ([]AgentInfo, error) {
	var agents []AgentInfo

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		agentPath := filepath.Join(dir, entry.Name())
		agent, err := parseAgentFile(agentPath)
		if err != nil {
			continue
		}
		if agent != nil {
			agents = append(agents, *agent)
		}
	}

	return agents, nil
}

// parseSkillFile reads YAML frontmatter from a SKILL.md file
func parseSkillFile(path string) (*SkillInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	return parseSkillFrontmatter(scanner), nil
}

// parseAgentFile reads YAML frontmatter from an agent .md file
func parseAgentFile(path string) (*AgentInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	return parseAgentFrontmatter(scanner), nil
}

// parseSkillFrontmatter parses YAML frontmatter from a scanner for skills
func parseSkillFrontmatter(scanner *bufio.Scanner) *SkillInfo {
	// Check for YAML frontmatter delimiter
	if !scanner.Scan() || scanner.Text() != "---" {
		return nil
	}

	skill := &SkillInfo{}
	inTriggers := false
	inDescription := false
	var descLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}

		// Handle multi-line description continuation
		if inDescription {
			if strings.HasPrefix(line, "  ") {
				descLines = append(descLines, strings.TrimSpace(line))
				continue
			}
			skill.Description = strings.Join(descLines, " ")
			inDescription = false
		}

		switch {
		case strings.HasPrefix(line, "name:"):
			skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))

		case strings.HasPrefix(line, "description:"):
			desc := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			if desc == ">" || desc == "|" {
				inDescription = true
				descLines = nil
			} else {
				skill.Description = desc
			}

		case strings.HasPrefix(line, "triggers:"):
			inTriggers = true

		case inTriggers && strings.HasPrefix(line, "  - "):
			trigger := strings.TrimPrefix(line, "  - ")
			trigger = strings.Trim(trigger, "\"'")
			skill.Triggers = append(skill.Triggers, trigger)

		case !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "-"):
			inTriggers = false
		}
	}

	// Finalize pending multi-line description
	if inDescription && len(descLines) > 0 {
		skill.Description = strings.Join(descLines, " ")
	}

	if skill.Name == "" {
		return nil
	}

	return skill
}

// parseAgentFrontmatter parses YAML frontmatter from a scanner for agents
func parseAgentFrontmatter(scanner *bufio.Scanner) *AgentInfo {
	// Check for YAML frontmatter delimiter
	if !scanner.Scan() || scanner.Text() != "---" {
		return nil
	}

	agent := &AgentInfo{}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}

		switch {
		case strings.HasPrefix(line, "name:"):
			agent.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))

		case strings.HasPrefix(line, "description:"):
			agent.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))

		case strings.HasPrefix(line, "model:"):
			agent.Model = strings.TrimSpace(strings.TrimPrefix(line, "model:"))
		}
	}

	if agent.Name == "" {
		return nil
	}

	return agent
}

// OutputClaudeContext writes available skills and agents to stdout for polecats/crew
func OutputClaudeContext(w io.Writer, role Role) error {
	// Only output for polecat and crew roles
	if role != RolePolecat && role != RoleCrew {
		return nil
	}

	skills, _ := DiscoverSkills()
	agents, _ := DiscoverAgents()

	// Nothing to output
	if len(skills) == 0 && len(agents) == 0 {
		return nil
	}

	bw := bufio.NewWriter(w)

	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "## Claude Code Assets")
	fmt.Fprintln(bw)
	fmt.Fprintln(bw, "Available skills and agents from `~/.claude/`:")
	fmt.Fprintln(bw)

	// Output skills table
	if len(skills) > 0 {
		fmt.Fprintln(bw, "### Skills")
		fmt.Fprintln(bw)
		fmt.Fprintln(bw, "Invoke with `/skill-name`:")
		fmt.Fprintln(bw)
		fmt.Fprintln(bw, "| Skill | Description |")
		fmt.Fprintln(bw, "|-------|-------------|")

		for _, s := range skills {
			desc := truncateDescription(s.Description, 60)
			fmt.Fprintf(bw, "| `/%s` | %s |\n", s.Name, desc)
		}
		fmt.Fprintln(bw)
	}

	// Output agents table
	if len(agents) > 0 {
		fmt.Fprintln(bw, "### Agents")
		fmt.Fprintln(bw)
		fmt.Fprintln(bw, "Invoke via Task() or explicit request:")
		fmt.Fprintln(bw)
		fmt.Fprintln(bw, "| Agent | Model | Description |")
		fmt.Fprintln(bw, "|-------|-------|-------------|")

		for _, a := range agents {
			desc := truncateDescription(a.Description, 50)
			model := a.Model
			if model == "" {
				model = "default"
			}
			fmt.Fprintf(bw, "| `%s` | %s | %s |\n", a.Name, model, desc)
		}
		fmt.Fprintln(bw)
	}

	return bw.Flush()
}

// truncateDescription shortens and cleans description for table display
func truncateDescription(desc string, maxLen int) string {
	// Clean up for table display
	desc = strings.ReplaceAll(desc, "|", "-")
	desc = strings.ReplaceAll(desc, "\n", " ")

	if len(desc) > maxLen {
		return desc[:maxLen-3] + "..."
	}
	return desc
}

// OutputSkillContext is kept for backward compatibility, calls OutputClaudeContext
func OutputSkillContext(w io.Writer, role Role) error {
	return OutputClaudeContext(w, role)
}

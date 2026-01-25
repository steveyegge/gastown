package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSkills(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test SKILL.md
	skillContent := `---
name: test-skill
description: A test skill for testing
triggers:
  - "test"
  - "testing"
---
# Test Skill
`
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override HOME for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	skills, err := DiscoverSkills()
	if err != nil {
		t.Fatalf("DiscoverSkills() error: %v", err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}

	if skills[0].Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got '%s'", skills[0].Name)
	}

	if skills[0].Description != "A test skill for testing" {
		t.Errorf("expected description 'A test skill for testing', got '%s'", skills[0].Description)
	}

	if len(skills[0].Triggers) != 2 {
		t.Errorf("expected 2 triggers, got %d", len(skills[0].Triggers))
	}
}

func TestDiscoverAgents(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test agent .md file
	agentContent := `---
name: test-agent
description: A test agent for testing
model: opus
---
# Test Agent
`
	if err := os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override HOME for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	agents, err := DiscoverAgents()
	if err != nil {
		t.Fatalf("DiscoverAgents() error: %v", err)
	}

	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	if agents[0].Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", agents[0].Name)
	}

	if agents[0].Description != "A test agent for testing" {
		t.Errorf("expected description 'A test agent for testing', got '%s'", agents[0].Description)
	}

	if agents[0].Model != "opus" {
		t.Errorf("expected model 'opus', got '%s'", agents[0].Model)
	}
}

func TestParseSkillFileMultilineDescription(t *testing.T) {
	tmpDir := t.TempDir()

	// Create skill with multi-line description
	skillContent := `---
name: multiline-skill
description: >
  This is a multi-line description
  that spans multiple lines.
triggers:
  - "multi"
---
# Multi-line Skill
`
	skillPath := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := parseSkillFile(skillPath)
	if err != nil {
		t.Fatalf("parseSkillFile() error: %v", err)
	}

	if skill == nil {
		t.Fatal("expected skill, got nil")
	}

	if skill.Name != "multiline-skill" {
		t.Errorf("expected name 'multiline-skill', got '%s'", skill.Name)
	}

	// Description should be joined
	if skill.Description == "" {
		t.Error("expected non-empty description")
	}
}

func TestOutputClaudeContext(t *testing.T) {
	// Create temp directory with skills and agents
	tmpDir := t.TempDir()

	// Create skills dir
	skillsDir := filepath.Join(tmpDir, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillContent := `---
name: test-skill
description: Test skill description
---
`
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create agents dir
	agentsDir := filepath.Join(tmpDir, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}
	agentContent := `---
name: test-agent
description: Test agent description
model: sonnet
---
`
	if err := os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Override HOME for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	var buf bytes.Buffer
	err := OutputClaudeContext(&buf, RolePolecat)
	if err != nil {
		t.Fatalf("OutputClaudeContext() error: %v", err)
	}

	output := buf.String()

	// Should contain skills section
	if !strings.Contains(output, "### Skills") {
		t.Error("expected output to contain '### Skills'")
	}

	// Should contain agents section
	if !strings.Contains(output, "### Agents") {
		t.Error("expected output to contain '### Agents'")
	}

	// Should contain our test skill
	if !strings.Contains(output, "test-skill") {
		t.Error("expected output to contain 'test-skill'")
	}

	// Should contain our test agent
	if !strings.Contains(output, "test-agent") {
		t.Error("expected output to contain 'test-agent'")
	}
}

func TestOutputClaudeContextMayorRole(t *testing.T) {
	// Create temp directory with skills
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claude", "skills", "test-skill")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillContent := `---
name: test-skill
description: Test skill
---
`
	if err := os.WriteFile(filepath.Join(skillsDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	var buf bytes.Buffer
	err := OutputClaudeContext(&buf, RoleMayor)
	if err != nil {
		t.Fatalf("OutputClaudeContext() error: %v", err)
	}

	// Mayor role should produce no output
	if buf.Len() != 0 {
		t.Errorf("expected empty output for Mayor role, got %d bytes", buf.Len())
	}
}

func TestGracefulDegradation(t *testing.T) {
	// Point HOME to non-existent directory
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/nonexistent-path-that-does-not-exist")
	defer os.Setenv("HOME", origHome)

	skills, err := DiscoverSkills()
	if err != nil {
		t.Fatalf("DiscoverSkills() should not error on missing dir: %v", err)
	}
	if skills != nil && len(skills) > 0 {
		t.Errorf("expected no skills, got %d", len(skills))
	}

	agents, err := DiscoverAgents()
	if err != nil {
		t.Fatalf("DiscoverAgents() should not error on missing dir: %v", err)
	}
	if agents != nil && len(agents) > 0 {
		t.Errorf("expected no agents, got %d", len(agents))
	}
}

func TestParseSkillFileNoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create skill without frontmatter
	skillContent := `# Just a Markdown File

No YAML frontmatter here.
`
	skillPath := filepath.Join(tmpDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	skill, err := parseSkillFile(skillPath)
	if err != nil {
		t.Fatalf("parseSkillFile() error: %v", err)
	}

	if skill != nil {
		t.Error("expected nil skill for file without frontmatter")
	}
}

func TestParseAgentFileNoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create agent without frontmatter
	agentContent := `# Just a Markdown File

No YAML frontmatter here.
`
	agentPath := filepath.Join(tmpDir, "agent.md")
	if err := os.WriteFile(agentPath, []byte(agentContent), 0644); err != nil {
		t.Fatal(err)
	}

	agent, err := parseAgentFile(agentPath)
	if err != nil {
		t.Fatalf("parseAgentFile() error: %v", err)
	}

	if agent != nil {
		t.Error("expected nil agent for file without frontmatter")
	}
}

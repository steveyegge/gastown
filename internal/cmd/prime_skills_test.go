package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverSkills(t *testing.T) {
	// Create temp skills directory
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claude", "cowork-skills", "test-skill")
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

func TestOutputSkillContext(t *testing.T) {
	var buf bytes.Buffer

	// With no skills (empty HOME), should output nothing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", "/nonexistent-dir-for-test")
	defer os.Setenv("HOME", origHome)

	err := OutputSkillContext(&buf, RolePolecat)
	if err != nil {
		t.Fatalf("OutputSkillContext() error: %v", err)
	}

	// Should be empty since no skills found
	if buf.Len() != 0 {
		t.Errorf("expected empty output, got %d bytes", buf.Len())
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
}

func TestIsRelevantForRole(t *testing.T) {
	tests := []struct {
		name     string
		skill    SkillInfo
		role     Role
		expected bool
	}{
		{
			name:     "implement skill for polecat",
			skill:    SkillInfo{Name: "implement"},
			role:     RolePolecat,
			expected: true,
		},
		{
			name:     "dispatch skill for crew",
			skill:    SkillInfo{Name: "dispatch"},
			role:     RoleCrew,
			expected: true,
		},
		{
			name:     "implement skill for mayor",
			skill:    SkillInfo{Name: "implement"},
			role:     RoleMayor,
			expected: false,
		},
		{
			name:     "unknown skill for polecat",
			skill:    SkillInfo{Name: "unknown-skill"},
			role:     RolePolecat,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRelevantForRole(tt.skill, tt.role)
			if result != tt.expected {
				t.Errorf("isRelevantForRole(%s, %s) = %v, want %v",
					tt.skill.Name, tt.role, result, tt.expected)
			}
		})
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

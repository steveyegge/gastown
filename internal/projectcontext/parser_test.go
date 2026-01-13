// Package projectcontext_test tests the projectcontext package.
package projectcontext_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/steveyegge/gastown/internal/projectcontext"
)

func TestParse(t *testing.T) {
	t.Run("parses CLAUDE.md into guidelines", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		claudeMDContent := "Guideline 1\nGuideline 2"
		claudeMDPath := filepath.Join(tmpDir, "CLAUDE.md")
		if err := os.WriteFile(claudeMDPath, []byte(claudeMDContent), 0644); err != nil {
			t.Fatalf("Failed to write CLAUDE.md: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		expectedGuidelines := []string{"Guideline 1", "Guideline 2"}
		if !reflect.DeepEqual(ctx.Guidelines, expectedGuidelines) {
			t.Errorf("Expected guidelines %v, got %v", expectedGuidelines, ctx.Guidelines)
		}
	})

	t.Run("parses .claude/commands into skills", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		commandsDir := filepath.Join(tmpDir, ".claude", "commands")
		if err := os.MkdirAll(commandsDir, 0755); err != nil {
			t.Fatalf("Failed to create commands dir: %v", err)
		}

		skillContent := "This is the intent.\nStep 1\nStep 2"
		skillPath := filepath.Join(commandsDir, "my-skill.sh")
		if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
			t.Fatalf("Failed to write skill file: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		expectedSkills := []projectcontext.Skill{
			{
				Name:   "my-skill",
				Intent: "This is the intent.",
				Steps:  []string{"Step 1", "Step 2"},
			},
		}

		if len(ctx.Skills) != 1 {
			t.Fatalf("Expected 1 skill, got %d", len(ctx.Skills))
		}

		if !reflect.DeepEqual(ctx.Skills, expectedSkills) {
			t.Errorf("Expected skills %v, got %v", expectedSkills, ctx.Skills)
		}
	})

	t.Run("parses .claude/rules into rules", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		rulesDir := filepath.Join(tmpDir, ".claude")
		if err := os.MkdirAll(rulesDir, 0755); err != nil {
			t.Fatalf("Failed to create .claude dir: %v", err)
		}

		rulesContent := "Rule 1\nRule 2"
		rulesPath := filepath.Join(rulesDir, "rules")
		if err := os.WriteFile(rulesPath, []byte(rulesContent), 0644); err != nil {
			t.Fatalf("Failed to write rules file: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		expectedRules := []string{"Rule 1", "Rule 2"}
		if !reflect.DeepEqual(ctx.Rules, expectedRules) {
			t.Errorf("Expected rules %v, got %v", expectedRules, ctx.Rules)
		}
	})

	t.Run("handles empty CLAUDE.md", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		claudeMDPath := filepath.Join(tmpDir, "CLAUDE.md")
		if err := os.WriteFile(claudeMDPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write CLAUDE.md: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		if len(ctx.Guidelines) != 0 {
			t.Errorf("Expected 0 guidelines, got %d", len(ctx.Guidelines))
		}
	})

	t.Run("handles empty .claude/commands directory", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		commandsDir := filepath.Join(tmpDir, ".claude", "commands")
		if err := os.MkdirAll(commandsDir, 0755); err != nil {
			t.Fatalf("Failed to create commands dir: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		if len(ctx.Skills) != 0 {
			t.Errorf("Expected 0 skills, got %d", len(ctx.Skills))
		}
	})

	t.Run("handles empty .claude/rules file", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		rulesDir := filepath.Join(tmpDir, ".claude")
		if err := os.MkdirAll(rulesDir, 0755); err != nil {
			t.Fatalf("Failed to create .claude dir: %v", err)
		}

		rulesPath := filepath.Join(rulesDir, "rules")
		if err := os.WriteFile(rulesPath, []byte(""), 0644); err != nil {
			t.Fatalf("Failed to write rules file: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		if len(ctx.Rules) != 0 {
			t.Errorf("Expected 0 rules, got %d", len(ctx.Rules))
		}
	})

	t.Run("parses all sources together", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "gastown-test")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		defer os.RemoveAll(tmpDir)

		// CLAUDE.md
		claudeMDContent := "Guideline 1"
		claudeMDPath := filepath.Join(tmpDir, "CLAUDE.md")
		if err := os.WriteFile(claudeMDPath, []byte(claudeMDContent), 0644); err != nil {
			t.Fatalf("Failed to write CLAUDE.md: %v", err)
		}

		// .claude/commands
		commandsDir := filepath.Join(tmpDir, ".claude", "commands")
		if err := os.MkdirAll(commandsDir, 0755); err != nil {
			t.Fatalf("Failed to create commands dir: %v", err)
		}
		skillContent := "Skill intent"
		skillPath := filepath.Join(commandsDir, "a-skill.sh")
		if err := os.WriteFile(skillPath, []byte(skillContent), 0644); err != nil {
			t.Fatalf("Failed to write skill file: %v", err)
		}

		// .claude/rules
		rulesDir := filepath.Join(tmpDir, ".claude")
		rulesContent := "Rule 1"
		rulesPath := filepath.Join(rulesDir, "rules")
		if err := os.WriteFile(rulesPath, []byte(rulesContent), 0644); err != nil {
			t.Fatalf("Failed to write rules file: %v", err)
		}

		ctx, err := projectcontext.Parse(tmpDir)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		if len(ctx.Guidelines) != 1 || ctx.Guidelines[0] != "Guideline 1" {
			t.Errorf("Unexpected guidelines: %v", ctx.Guidelines)
		}
		if len(ctx.Skills) != 1 || ctx.Skills[0].Name != "a-skill" {
			t.Errorf("Unexpected skills: %v", ctx.Skills)
		}
		if len(ctx.Rules) != 1 || ctx.Rules[0] != "Rule 1" {
			t.Errorf("Unexpected rules: %v", ctx.Rules)
		}
	})
}

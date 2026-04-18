package commands

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildCommand_Claude(t *testing.T) {
	cmd := FindByName("handoff")
	if cmd == nil {
		t.Fatal("handoff command not found")
	}
	content, err := BuildCommand(*cmd, "claude")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	// Check frontmatter
	if !strings.Contains(content, "description: Hand off to fresh session") {
		t.Error("missing description")
	}
	if !strings.Contains(content, "allowed-tools: Bash(gt handoff:*)") {
		t.Error("missing allowed-tools for Claude")
	}
	if !strings.Contains(content, "argument-hint: [message]") {
		t.Error("missing argument-hint for Claude")
	}

	// Check body
	if !strings.Contains(content, "$ARGUMENTS") {
		t.Error("missing $ARGUMENTS in body")
	}
	if strings.Contains(content, "Ready to hand off?") {
		t.Error("Claude handoff command should not require an interactive confirmation prompt")
	}
	if !strings.Contains(content, "gt handoff -y") {
		t.Error("Claude handoff command should use gt handoff -y")
	}
}

func TestBuildCommand_OpenCode(t *testing.T) {
	cmd := FindByName("handoff")
	if cmd == nil {
		t.Fatal("handoff command not found")
	}
	content, err := BuildCommand(*cmd, "opencode")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	// Check frontmatter - only description, no Claude-specific fields
	if !strings.Contains(content, "description: Hand off to fresh session") {
		t.Error("missing description")
	}
	if strings.Contains(content, "allowed-tools") {
		t.Error("OpenCode should not have allowed-tools")
	}
	if strings.Contains(content, "argument-hint") {
		t.Error("OpenCode should not have argument-hint")
	}

	// Check body
	if !strings.Contains(content, "$ARGUMENTS") {
		t.Error("missing $ARGUMENTS in body")
	}
}

func TestBuildCommand_Copilot(t *testing.T) {
	cmd := FindByName("handoff")
	if cmd == nil {
		t.Fatal("handoff command not found")
	}
	content, err := BuildCommand(*cmd, "copilot")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	// Check frontmatter - only description, no Claude-specific fields
	if !strings.Contains(content, "description: Hand off to fresh session") {
		t.Error("missing description")
	}
	if strings.Contains(content, "allowed-tools") {
		t.Error("Copilot should not have allowed-tools")
	}
	if strings.Contains(content, "argument-hint") {
		t.Error("Copilot should not have argument-hint")
	}

	// Check body
	if !strings.Contains(content, "$ARGUMENTS") {
		t.Error("missing $ARGUMENTS in body")
	}
}

func TestBuildCommand_Review_Claude(t *testing.T) {
	cmd := FindByName("review")
	if cmd == nil {
		t.Fatal("review command not found")
	}
	content, err := BuildCommand(*cmd, "claude")
	if err != nil {
		t.Fatalf("BuildCommand failed: %v", err)
	}

	// Check frontmatter
	if !strings.Contains(content, "description: Review code changes with structured grading") {
		t.Error("missing description")
	}
	if !strings.Contains(content, "allowed-tools:") {
		t.Error("missing allowed-tools for Claude")
	}
	if !strings.Contains(content, "argument-hint:") {
		t.Error("missing argument-hint for Claude")
	}

	// Check body
	if !strings.Contains(content, "$ARGUMENTS") {
		t.Error("missing $ARGUMENTS in body")
	}
	if !strings.Contains(content, "CRITICAL") {
		t.Error("missing CRITICAL severity in body")
	}
	if !strings.Contains(content, "Grade") {
		t.Error("missing Grade in body")
	}
}

func TestNames(t *testing.T) {
	names := Names()
	if len(names) < 2 {
		t.Errorf("expected at least 2 commands, got %d", len(names))
	}
	if !slices.Contains(names, "handoff") {
		t.Error("missing handoff command")
	}
	if !slices.Contains(names, "review") {
		t.Error("missing review command")
	}
}

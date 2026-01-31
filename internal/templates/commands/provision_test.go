package commands

import (
	"strings"
	"testing"
)

func TestBuildCommand_Claude(t *testing.T) {
	cmd := Commands[0] // handoff
	content, err := BuildCommand(cmd, "claude")
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
}

func TestBuildCommand_OpenCode(t *testing.T) {
	cmd := Commands[0] // handoff
	content, err := BuildCommand(cmd, "opencode")
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

func TestNames(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Error("no commands registered")
	}
	if names[0] != "handoff" {
		t.Errorf("expected handoff, got %s", names[0])
	}
}

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

func TestBuildCommand_Copilot(t *testing.T) {
	cmd := Commands[0] // handoff
	content, err := BuildCommand(cmd, "copilot")
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

func TestNames(t *testing.T) {
	names := Names()
	if len(names) == 0 {
		t.Error("no commands registered")
	}
	if names[0] != "handoff" {
		t.Errorf("expected handoff, got %s", names[0])
	}
}

func TestIsKnownAgent(t *testing.T) {
	tests := []struct {
		agent string
		want  bool
	}{
		{"claude", true},
		{"Claude", true}, // case insensitive
		{"opencode", true},
		{"copilot", true},
		{"nonexistent", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			if got := IsKnownAgent(tt.agent); got != tt.want {
				t.Errorf("IsKnownAgent(%q) = %v, want %v", tt.agent, got, tt.want)
			}
		})
	}
}

func TestProvisionFor(t *testing.T) {
	dir := t.TempDir()

	if err := ProvisionFor(dir, "claude"); err != nil {
		t.Fatalf("ProvisionFor() error = %v", err)
	}

	// Verify commands were created
	missing := MissingFor(dir, "claude")
	if len(missing) != 0 {
		t.Errorf("MissingFor() = %v after provisioning", missing)
	}
}

func TestProvisionFor_NoOverwrite(t *testing.T) {
	dir := t.TempDir()

	// First provision
	if err := ProvisionFor(dir, "claude"); err != nil {
		t.Fatal(err)
	}

	// Second provision should not error (skips existing)
	if err := ProvisionFor(dir, "claude"); err != nil {
		t.Fatalf("second ProvisionFor() error = %v", err)
	}
}

func TestProvisionFor_UnknownAgent(t *testing.T) {
	dir := t.TempDir()
	err := ProvisionFor(dir, "unknown-agent")
	if err == nil {
		t.Error("ProvisionFor(unknown) should error")
	}
}

func TestMissingFor_AllMissing(t *testing.T) {
	dir := t.TempDir()
	missing := MissingFor(dir, "claude")
	if len(missing) != len(Commands) {
		t.Errorf("MissingFor() = %d, want %d", len(missing), len(Commands))
	}
}

func TestMissingFor_UnknownAgent(t *testing.T) {
	dir := t.TempDir()
	missing := MissingFor(dir, "unknown-agent")
	if missing != nil {
		t.Errorf("MissingFor(unknown) = %v, want nil", missing)
	}
}

func TestProvisionFor_MultipleAgents(t *testing.T) {
	dir := t.TempDir()

	for _, agent := range []string{"claude", "opencode", "copilot"} {
		t.Run(agent, func(t *testing.T) {
			if !IsKnownAgent(agent) {
				t.Skipf("%s not a known agent", agent)
			}
			if err := ProvisionFor(dir, agent); err != nil {
				t.Fatalf("ProvisionFor(%s) error = %v", agent, err)
			}
			missing := MissingFor(dir, agent)
			if len(missing) != 0 {
				t.Errorf("MissingFor(%s) = %v", agent, missing)
			}
		})
	}
}

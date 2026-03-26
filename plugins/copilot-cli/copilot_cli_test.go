package copilot_cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pluginDir returns the absolute path to the copilot-cli plugin directory.
func pluginDir(t *testing.T) string {
	t.Helper()
	// This test file lives in plugins/copilot-cli/, so use its directory.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(thisFile)
}

func TestPluginStructure(t *testing.T) {
	t.Parallel()
	root := pluginDir(t)

	required := []string{
		"install.sh",
		"install.ps1",
		"mcp-config-fragment.json",
		filepath.Join("skills", "gastown", "SKILL.md"),
		filepath.Join("skills", "gastown", "references", "polecat-lifecycle.md"),
		filepath.Join("skills", "gastown", "references", "mail-protocol.md"),
		filepath.Join("skills", "gastown", "references", "issue-workflow.md"),
		filepath.Join("skills", "gastown", "references", "landing-the-plane.md"),
		filepath.Join("agents", "gastown-crew.md"),
	}

	for _, rel := range required {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("required file missing: %s", rel)
		}
	}
}

func TestMCPConfigFragment(t *testing.T) {
	t.Parallel()
	root := pluginDir(t)
	data, err := os.ReadFile(filepath.Join(root, "mcp-config-fragment.json"))
	if err != nil {
		t.Fatalf("reading mcp-config-fragment.json: %v", err)
	}

	var config struct {
		MCPServers map[string]struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	srv, ok := config.MCPServers["gastown-hooks"]
	if !ok {
		t.Fatal("mcp-config-fragment.json missing 'gastown-hooks' server entry")
	}
	if srv.Type != "http" {
		t.Errorf("gastown-hooks type = %q, want http", srv.Type)
	}
	if !strings.Contains(srv.URL, "/mcp") {
		t.Errorf("gastown-hooks URL = %q, should contain /mcp", srv.URL)
	}
}

func TestSKILLMDContent(t *testing.T) {
	t.Parallel()
	root := pluginDir(t)
	data, err := os.ReadFile(filepath.Join(root, "skills", "gastown", "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	content := string(data)

	// Check YAML frontmatter (handle both LF and CRLF line endings)
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		t.Error("SKILL.md should start with YAML frontmatter (---)")
	}

	// Mandatory sections
	sections := []string{
		"# Gas Town",
		"## Prerequisites",
		"## Quick Reference",
		"## Polecat Workflow",
		"## Gas Town MCP Tools",
		"## References",
	}
	for _, s := range sections {
		if !strings.Contains(content, s) {
			t.Errorf("SKILL.md missing section: %s", s)
		}
	}

	// Verify commands match actual CLI (regression guard)
	mustContain := []string{
		"gt mail inbox", // NOT "gt mail check" (that's for hooks)
		"bd ready",      // standalone bd command
		"bd update",     // standalone bd command
		"gt done",       // completion signal
		"gt prime",      // context recovery
	}
	for _, cmd := range mustContain {
		if !strings.Contains(content, cmd) {
			t.Errorf("SKILL.md missing command reference: %s", cmd)
		}
	}

	// Verify NO stale commands
	stale := []string{
		"gt mail check", // Should be gt mail inbox for inbox viewing
		"gt bd ready",   // bd is standalone, not gt subcommand
		"gt bd update",  // bd is standalone
		"gt bd close",   // bd is standalone
	}
	for _, cmd := range stale {
		if strings.Contains(content, cmd) {
			t.Errorf("SKILL.md contains stale command: %s", cmd)
		}
	}
}

func TestAgentProfileContent(t *testing.T) {
	t.Parallel()
	root := pluginDir(t)
	data, err := os.ReadFile(filepath.Join(root, "agents", "gastown-crew.md"))
	if err != nil {
		t.Fatalf("reading gastown-crew.md: %v", err)
	}
	content := string(data)

	// Check YAML frontmatter (handle both LF and CRLF line endings)
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		t.Error("gastown-crew.md should start with YAML frontmatter (---)")
	}
	if !strings.Contains(content, "name: gastown-crew") {
		t.Error("gastown-crew.md missing name in frontmatter")
	}

	// Critical workflow rules
	rules := []string{
		"gt done",
		"gt mail",
		"bd ready",
		"Startup Sequence",
		"Completion",
	}
	for _, r := range rules {
		if !strings.Contains(content, r) {
			t.Errorf("gastown-crew.md missing: %s", r)
		}
	}
}

func TestReferenceDocsExist(t *testing.T) {
	t.Parallel()
	root := pluginDir(t)
	refDir := filepath.Join(root, "skills", "gastown", "references")

	refs := map[string][]string{
		"polecat-lifecycle.md": {"WORKING", "IDLE", "DONE", "STUCK"},
		"mail-protocol.md":     {"gt mail send", "gt mail inbox", "Nudge"},
		"issue-workflow.md":    {"bd ready", "bd create", "bd update", "bd close"},
		"landing-the-plane.md": {"gt done", "git push", "MANDATORY"},
	}

	for file, keywords := range refs {
		data, err := os.ReadFile(filepath.Join(refDir, file))
		if err != nil {
			t.Errorf("reading %s: %v", file, err)
			continue
		}
		content := string(data)
		for _, kw := range keywords {
			if !strings.Contains(content, kw) {
				t.Errorf("%s missing keyword: %s", file, kw)
			}
		}
	}
}

func TestInstallScriptsExist(t *testing.T) {
	t.Parallel()
	root := pluginDir(t)

	// Bash installer
	bashData, err := os.ReadFile(filepath.Join(root, "install.sh"))
	if err != nil {
		t.Fatalf("reading install.sh: %v", err)
	}
	bashContent := string(bashData)
	if !strings.HasPrefix(bashContent, "#!/usr/bin/env bash") {
		t.Error("install.sh missing shebang")
	}
	if !strings.Contains(bashContent, "COPILOT_HOME") {
		t.Error("install.sh missing COPILOT_HOME reference")
	}
	if !strings.Contains(bashContent, "--check") {
		t.Error("install.sh missing --check mode")
	}

	// PowerShell installer
	psData, err := os.ReadFile(filepath.Join(root, "install.ps1"))
	if err != nil {
		t.Fatalf("reading install.ps1: %v", err)
	}
	psContent := string(psData)
	if !strings.Contains(psContent, "COPILOT_HOME") {
		t.Error("install.ps1 missing COPILOT_HOME reference")
	}
	if !strings.Contains(psContent, "-Check") {
		t.Error("install.ps1 missing -Check parameter")
	}
	// Cross-platform: should use $HOME not $env:USERPROFILE in code lines
	for i, line := range strings.Split(psContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue // skip comments
		}
		if strings.Contains(line, "$env:USERPROFILE") {
			t.Errorf("install.ps1 line %d uses $env:USERPROFILE instead of $HOME (not cross-platform)", i+1)
		}
	}
}

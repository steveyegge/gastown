package ui

import (
	"os"
	"strings"
	"testing"
)

func TestRenderMarkdown_AgentMode(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
	}()

	os.Setenv("GT_AGENT_MODE", "1")

	markdown := "# Test Header\n\nSome content"
	result := RenderMarkdown(markdown)

	if result != markdown {
		t.Errorf("RenderMarkdown() in agent mode should return raw markdown, got %q", result)
	}
}

func TestRenderMarkdown_SimpleText(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1") // Disable glamour rendering

	markdown := "Simple text without formatting"
	result := RenderMarkdown(markdown)

	// When color is disabled, should return raw markdown
	if result != markdown {
		t.Errorf("RenderMarkdown() with color disabled should return raw markdown, got %q", result)
	}
}

func TestRenderMarkdown_EmptyString(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1")

	result := RenderMarkdown("")
	if result != "" {
		t.Errorf("RenderMarkdown() with empty string should return empty, got %q", result)
	}
}

func TestRenderMarkdown_GracefulDegradation(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1")

	// Test that function doesn't panic and always returns something
	markdown := "# Test\n\nContent with **bold** and *italic*"
	result := RenderMarkdown(markdown)

	if result == "" {
		t.Error("RenderMarkdown() should never return empty string for non-empty input")
	}
	// With NO_COLOR, should return raw markdown
	if !strings.Contains(result, "bold") {
		t.Error("RenderMarkdown() should contain original content")
	}
}

func TestGetTerminalWidth(t *testing.T) {
	// This function is unexported, but we can test it indirectly
	// The function should return a reasonable width
	// Since we can't call it directly, we verify RenderMarkdown doesn't panic

	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1")

	// This should not panic even if terminal width detection fails
	markdown := strings.Repeat("word ", 1000) // Long content
	result := RenderMarkdown(markdown)

	if result == "" {
		t.Error("RenderMarkdown() should handle long content")
	}
}

func TestRenderMarkdown_Newlines(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1")

	markdown := "Line 1\n\nLine 2\n\nLine 3"
	result := RenderMarkdown(markdown)

	// With color disabled, newlines should be preserved
	if !strings.Contains(result, "Line 1") {
		t.Error("RenderMarkdown() should preserve first line")
	}
	if !strings.Contains(result, "Line 2") {
		t.Error("RenderMarkdown() should preserve second line")
	}
	if !strings.Contains(result, "Line 3") {
		t.Error("RenderMarkdown() should preserve third line")
	}
}

func TestRenderMarkdown_CodeBlocks(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1")

	markdown := "```go\nfunc main() {}\n```"
	result := RenderMarkdown(markdown)

	// Should contain the code
	if !strings.Contains(result, "func main") {
		t.Error("RenderMarkdown() should preserve code block content")
	}
}

func TestRenderMarkdown_Links(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("NO_COLOR", "1")

	markdown := "[link text](https://example.com)"
	result := RenderMarkdown(markdown)

	// Should contain the link text or URL
	if !strings.Contains(result, "link text") && !strings.Contains(result, "example.com") {
		t.Error("RenderMarkdown() should preserve link information")
	}
}

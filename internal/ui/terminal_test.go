package ui

import (
	"os"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	// This test verifies the function doesn't panic
	// The actual result depends on the test environment
	result := IsTerminal()
	// In test environment, this is usually false
	// The important thing is it doesn't crash and returns a bool
	var _ bool = result
}

func TestShouldUseColor_Default(t *testing.T) {
	// Clean environment for this test
	oldNoColor := os.Getenv("NO_COLOR")
	oldClicolor := os.Getenv("CLICOLOR")
	oldClicolorForce := os.Getenv("CLICOLOR_FORCE")
	defer func() {
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if oldClicolor != "" {
			os.Setenv("CLICOLOR", oldClicolor)
		} else {
			os.Unsetenv("CLICOLOR")
		}
		if oldClicolorForce != "" {
			os.Setenv("CLICOLOR_FORCE", oldClicolorForce)
		} else {
			os.Unsetenv("CLICOLOR_FORCE")
		}
	}()

	os.Unsetenv("NO_COLOR")
	os.Unsetenv("CLICOLOR")
	os.Unsetenv("CLICOLOR_FORCE")

	result := ShouldUseColor()
	// In non-TTY test environment, should be false
	_ = result
}

func TestShouldUseColor_NO_COLOR(t *testing.T) {
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	os.Setenv("NO_COLOR", "1")
	if ShouldUseColor() {
		t.Error("ShouldUseColor() should return false when NO_COLOR is set")
	}
}

func TestShouldUseColor_NO_COLOR_AnyValue(t *testing.T) {
	oldNoColor := os.Getenv("NO_COLOR")
	defer func() {
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
	}()

	// NO_COLOR with any value (even "0") should disable color
	os.Setenv("NO_COLOR", "0")
	if ShouldUseColor() {
		t.Error("ShouldUseColor() should return false when NO_COLOR is set to any value")
	}
}

func TestShouldUseColor_CLICOLOR_0(t *testing.T) {
	oldNoColor := os.Getenv("NO_COLOR")
	oldClicolor := os.Getenv("CLICOLOR")
	defer func() {
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if oldClicolor != "" {
			os.Setenv("CLICOLOR", oldClicolor)
		} else {
			os.Unsetenv("CLICOLOR")
		}
	}()

	os.Unsetenv("NO_COLOR")
	os.Setenv("CLICOLOR", "0")
	if ShouldUseColor() {
		t.Error("ShouldUseColor() should return false when CLICOLOR=0")
	}
}

func TestShouldUseColor_CLICOLOR_FORCE(t *testing.T) {
	oldNoColor := os.Getenv("NO_COLOR")
	oldClicolorForce := os.Getenv("CLICOLOR_FORCE")
	defer func() {
		if oldNoColor != "" {
			os.Setenv("NO_COLOR", oldNoColor)
		} else {
			os.Unsetenv("NO_COLOR")
		}
		if oldClicolorForce != "" {
			os.Setenv("CLICOLOR_FORCE", oldClicolorForce)
		} else {
			os.Unsetenv("CLICOLOR_FORCE")
		}
	}()

	os.Unsetenv("NO_COLOR")
	os.Setenv("CLICOLOR_FORCE", "1")
	if !ShouldUseColor() {
		t.Error("ShouldUseColor() should return true when CLICOLOR_FORCE is set")
	}
}

func TestShouldUseEmoji_Default(t *testing.T) {
	oldNoEmoji := os.Getenv("GT_NO_EMOJI")
	defer func() {
		if oldNoEmoji != "" {
			os.Setenv("GT_NO_EMOJI", oldNoEmoji)
		} else {
			os.Unsetenv("GT_NO_EMOJI")
		}
	}()

	os.Unsetenv("GT_NO_EMOJI")
	result := ShouldUseEmoji()
	_ = result // Result depends on test environment
}

func TestShouldUseEmoji_GT_NO_EMOJI(t *testing.T) {
	oldNoEmoji := os.Getenv("GT_NO_EMOJI")
	defer func() {
		if oldNoEmoji != "" {
			os.Setenv("GT_NO_EMOJI", oldNoEmoji)
		} else {
			os.Unsetenv("GT_NO_EMOJI")
		}
	}()

	os.Setenv("GT_NO_EMOJI", "1")
	if ShouldUseEmoji() {
		t.Error("ShouldUseEmoji() should return false when GT_NO_EMOJI is set")
	}
}

func TestIsAgentMode_Default(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldClaudeCode := os.Getenv("CLAUDECODE")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldClaudeCode != "" {
			os.Setenv("CLAUDECODE", oldClaudeCode)
		} else {
			os.Unsetenv("CLAUDECODE")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Unsetenv("CLAUDECODE")
	if IsAgentMode() {
		t.Error("IsAgentMode() should return false by default")
	}
}

func TestIsAgentMode_GT_AGENT_MODE(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldClaudeCode := os.Getenv("CLAUDECODE")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldClaudeCode != "" {
			os.Setenv("CLAUDECODE", oldClaudeCode)
		} else {
			os.Unsetenv("CLAUDECODE")
		}
	}()

	// Unset CLAUDECODE to isolate GT_AGENT_MODE testing
	os.Unsetenv("CLAUDECODE")

	os.Setenv("GT_AGENT_MODE", "1")
	if !IsAgentMode() {
		t.Error("IsAgentMode() should return true when GT_AGENT_MODE=1")
	}

	os.Setenv("GT_AGENT_MODE", "0")
	if IsAgentMode() {
		t.Error("IsAgentMode() should return false when GT_AGENT_MODE=0")
	}
}

func TestIsAgentMode_CLAUDECODE(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldClaudeCode := os.Getenv("CLAUDECODE")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldClaudeCode != "" {
			os.Setenv("CLAUDECODE", oldClaudeCode)
		} else {
			os.Unsetenv("CLAUDECODE")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("CLAUDECODE", "1")
	if !IsAgentMode() {
		t.Error("IsAgentMode() should return true when CLAUDECODE is set")
	}
}

func TestIsAgentMode_CLAUDECODE_AnyValue(t *testing.T) {
	oldAgentMode := os.Getenv("GT_AGENT_MODE")
	oldClaudeCode := os.Getenv("CLAUDECODE")
	defer func() {
		if oldAgentMode != "" {
			os.Setenv("GT_AGENT_MODE", oldAgentMode)
		} else {
			os.Unsetenv("GT_AGENT_MODE")
		}
		if oldClaudeCode != "" {
			os.Setenv("CLAUDECODE", oldClaudeCode)
		} else {
			os.Unsetenv("CLAUDECODE")
		}
	}()

	os.Unsetenv("GT_AGENT_MODE")
	os.Setenv("CLAUDECODE", "any-value")
	if !IsAgentMode() {
		t.Error("IsAgentMode() should return true when CLAUDECODE is set to any value")
	}
}

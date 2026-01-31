package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gastown/internal/config"
)

func TestSessionIDFromEnv_Default(t *testing.T) {
	// Clear all environment variables
	oldGSEnv := os.Getenv("GT_SESSION_ID_ENV")
	oldClaudeID := os.Getenv("CLAUDE_SESSION_ID")
	defer func() {
		if oldGSEnv != "" {
			os.Setenv("GT_SESSION_ID_ENV", oldGSEnv)
		} else {
			os.Unsetenv("GT_SESSION_ID_ENV")
		}
		if oldClaudeID != "" {
			os.Setenv("CLAUDE_SESSION_ID", oldClaudeID)
		} else {
			os.Unsetenv("CLAUDE_SESSION_ID")
		}
	}()
	os.Unsetenv("GT_SESSION_ID_ENV")
	os.Unsetenv("CLAUDE_SESSION_ID")

	// Change to a temp dir with no .runtime/session_id file
	tmpDir := t.TempDir()
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	result := SessionIDFromEnv()
	if result != "" {
		t.Errorf("SessionIDFromEnv() with no env vars and no persisted file should return empty, got %q", result)
	}
}

func TestSessionIDFromEnv_ClaudeSessionID(t *testing.T) {
	oldGSEnv := os.Getenv("GT_SESSION_ID_ENV")
	oldClaudeID := os.Getenv("CLAUDE_SESSION_ID")
	defer func() {
		if oldGSEnv != "" {
			os.Setenv("GT_SESSION_ID_ENV", oldGSEnv)
		} else {
			os.Unsetenv("GT_SESSION_ID_ENV")
		}
		if oldClaudeID != "" {
			os.Setenv("CLAUDE_SESSION_ID", oldClaudeID)
		} else {
			os.Unsetenv("CLAUDE_SESSION_ID")
		}
	}()

	os.Unsetenv("GT_SESSION_ID_ENV")
	os.Setenv("CLAUDE_SESSION_ID", "test-session-123")

	result := SessionIDFromEnv()
	if result != "test-session-123" {
		t.Errorf("SessionIDFromEnv() = %q, want %q", result, "test-session-123")
	}
}

func TestSessionIDFromEnv_CustomEnvVar(t *testing.T) {
	oldGSEnv := os.Getenv("GT_SESSION_ID_ENV")
	oldCustomID := os.Getenv("CUSTOM_SESSION_ID")
	oldClaudeID := os.Getenv("CLAUDE_SESSION_ID")
	defer func() {
		if oldGSEnv != "" {
			os.Setenv("GT_SESSION_ID_ENV", oldGSEnv)
		} else {
			os.Unsetenv("GT_SESSION_ID_ENV")
		}
		if oldCustomID != "" {
			os.Setenv("CUSTOM_SESSION_ID", oldCustomID)
		} else {
			os.Unsetenv("CUSTOM_SESSION_ID")
		}
		if oldClaudeID != "" {
			os.Setenv("CLAUDE_SESSION_ID", oldClaudeID)
		} else {
			os.Unsetenv("CLAUDE_SESSION_ID")
		}
	}()

	os.Setenv("GT_SESSION_ID_ENV", "CUSTOM_SESSION_ID")
	os.Setenv("CUSTOM_SESSION_ID", "custom-session-456")
	os.Setenv("CLAUDE_SESSION_ID", "claude-session-789")

	result := SessionIDFromEnv()
	if result != "custom-session-456" {
		t.Errorf("SessionIDFromEnv() with custom env = %q, want %q", result, "custom-session-456")
	}
}

func TestSessionIDFromEnv_PersistedFile(t *testing.T) {
	// Clear env vars so we fall through to file-based lookup
	oldGSEnv := os.Getenv("GT_SESSION_ID_ENV")
	oldClaudeID := os.Getenv("CLAUDE_SESSION_ID")
	defer func() {
		if oldGSEnv != "" {
			os.Setenv("GT_SESSION_ID_ENV", oldGSEnv)
		} else {
			os.Unsetenv("GT_SESSION_ID_ENV")
		}
		if oldClaudeID != "" {
			os.Setenv("CLAUDE_SESSION_ID", oldClaudeID)
		} else {
			os.Unsetenv("CLAUDE_SESSION_ID")
		}
	}()
	os.Unsetenv("GT_SESSION_ID_ENV")
	os.Unsetenv("CLAUDE_SESSION_ID")

	// Create a temp dir with .runtime/session_id
	tmpDir := t.TempDir()
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(runtimeDir, "session_id")
	if err := os.WriteFile(sessionFile, []byte(fmt.Sprintf("persisted-session-abc\n2026-01-28T00:00:00Z\n")), 0644); err != nil {
		t.Fatal(err)
	}

	// Change to that directory so readPersistedSessionID finds it
	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	result := SessionIDFromEnv()
	if result != "persisted-session-abc" {
		t.Errorf("SessionIDFromEnv() with persisted file = %q, want %q", result, "persisted-session-abc")
	}
}

func TestSessionIDFromEnv_EnvTakesPrecedenceOverFile(t *testing.T) {
	oldGSEnv := os.Getenv("GT_SESSION_ID_ENV")
	oldClaudeID := os.Getenv("CLAUDE_SESSION_ID")
	defer func() {
		if oldGSEnv != "" {
			os.Setenv("GT_SESSION_ID_ENV", oldGSEnv)
		} else {
			os.Unsetenv("GT_SESSION_ID_ENV")
		}
		if oldClaudeID != "" {
			os.Setenv("CLAUDE_SESSION_ID", oldClaudeID)
		} else {
			os.Unsetenv("CLAUDE_SESSION_ID")
		}
	}()
	os.Unsetenv("GT_SESSION_ID_ENV")
	os.Setenv("CLAUDE_SESSION_ID", "env-session-xyz")

	// Create a temp dir with .runtime/session_id (should NOT be used)
	tmpDir := t.TempDir()
	runtimeDir := filepath.Join(tmpDir, ".runtime")
	os.MkdirAll(runtimeDir, 0755)
	os.WriteFile(filepath.Join(runtimeDir, "session_id"), []byte("file-session-should-not-be-used\n"), 0644)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	result := SessionIDFromEnv()
	if result != "env-session-xyz" {
		t.Errorf("SessionIDFromEnv() should prefer env over file, got %q, want %q", result, "env-session-xyz")
	}
}

func TestSleepForReadyDelay_NilConfig(t *testing.T) {
	// Should not panic with nil config
	SleepForReadyDelay(nil)
}

func TestSleepForReadyDelay_ZeroDelay(t *testing.T) {
	rc := &config.RuntimeConfig{
		Tmux: &config.RuntimeTmuxConfig{
			ReadyDelayMs: 0,
		},
	}

	start := time.Now()
	SleepForReadyDelay(rc)
	elapsed := time.Since(start)

	// Should return immediately
	if elapsed > 100*time.Millisecond {
		t.Errorf("SleepForReadyDelay() with zero delay took too long: %v", elapsed)
	}
}

func TestSleepForReadyDelay_WithDelay(t *testing.T) {
	rc := &config.RuntimeConfig{
		Tmux: &config.RuntimeTmuxConfig{
			ReadyDelayMs: 10, // 10ms delay
		},
	}

	start := time.Now()
	SleepForReadyDelay(rc)
	elapsed := time.Since(start)

	// Should sleep for at least 10ms
	if elapsed < 10*time.Millisecond {
		t.Errorf("SleepForReadyDelay() should sleep for at least 10ms, took %v", elapsed)
	}
	// But not too long
	if elapsed > 50*time.Millisecond {
		t.Errorf("SleepForReadyDelay() slept too long: %v", elapsed)
	}
}

func TestSleepForReadyDelay_NilTmuxConfig(t *testing.T) {
	rc := &config.RuntimeConfig{
		Tmux: nil,
	}

	start := time.Now()
	SleepForReadyDelay(rc)
	elapsed := time.Since(start)

	// Should return immediately
	if elapsed > 100*time.Millisecond {
		t.Errorf("SleepForReadyDelay() with nil Tmux config took too long: %v", elapsed)
	}
}

func TestStartupFallbackCommands_NoHooks(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	commands := StartupFallbackCommands("polecat", rc)
	if commands == nil {
		t.Error("StartupFallbackCommands() with no hooks should return commands")
	}
	if len(commands) == 0 {
		t.Error("StartupFallbackCommands() should return at least one command")
	}
}

func TestStartupFallbackCommands_WithHooks(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "claude",
		},
	}

	commands := StartupFallbackCommands("polecat", rc)
	if commands != nil {
		t.Error("StartupFallbackCommands() with hooks provider should return nil")
	}
}

func TestStartupFallbackCommands_NilConfig(t *testing.T) {
	// Nil config defaults to claude provider, which has hooks
	// So it returns nil (no fallback commands needed)
	commands := StartupFallbackCommands("polecat", nil)
	if commands != nil {
		t.Error("StartupFallbackCommands() with nil config should return nil (defaults to claude with hooks)")
	}
}

func TestStartupFallbackCommands_AutonomousRole(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	autonomousRoles := []string{"polecat", "witness", "refinery", "deacon"}
	for _, role := range autonomousRoles {
		t.Run(role, func(t *testing.T) {
			commands := StartupFallbackCommands(role, rc)
			if commands == nil || len(commands) == 0 {
				t.Error("StartupFallbackCommands() should return commands for autonomous role")
			}
			// Should contain mail check
			found := false
			for _, cmd := range commands {
				if contains(cmd, "mail check --inject") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Commands for %s should contain mail check --inject", role)
			}
		})
	}
}

func TestStartupFallbackCommands_NonAutonomousRole(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	nonAutonomousRoles := []string{"mayor", "crew", "keeper"}
	for _, role := range nonAutonomousRoles {
		t.Run(role, func(t *testing.T) {
			commands := StartupFallbackCommands(role, rc)
			if commands == nil || len(commands) == 0 {
				t.Error("StartupFallbackCommands() should return commands for non-autonomous role")
			}
			// Should NOT contain mail check
			for _, cmd := range commands {
				if contains(cmd, "mail check --inject") {
					t.Errorf("Commands for %s should NOT contain mail check --inject", role)
				}
			}
		})
	}
}

func TestStartupFallbackCommands_RoleCasing(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	// Role should be lowercased internally
	commands := StartupFallbackCommands("POLECAT", rc)
	if commands == nil {
		t.Error("StartupFallbackCommands() should handle uppercase role")
	}
}

func TestEnsureSettingsForRole_NilConfig(t *testing.T) {
	// Should not panic with nil config
	err := EnsureSettingsForRole("/tmp/test", "polecat", nil)
	if err != nil {
		t.Errorf("EnsureSettingsForRole() with nil config should not error, got %v", err)
	}
}

func TestEnsureSettingsForRole_NilHooks(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: nil,
	}

	err := EnsureSettingsForRole("/tmp/test", "polecat", rc)
	if err != nil {
		t.Errorf("EnsureSettingsForRole() with nil hooks should not error, got %v", err)
	}
}

func TestEnsureSettingsForRole_UnknownProvider(t *testing.T) {
	rc := &config.RuntimeConfig{
		Hooks: &config.RuntimeHooksConfig{
			Provider: "unknown",
		},
	}

	err := EnsureSettingsForRole("/tmp/test", "polecat", rc)
	if err != nil {
		t.Errorf("EnsureSettingsForRole() with unknown provider should not error, got %v", err)
	}
}

func TestGetStartupFallbackInfo_HooksWithPrompt(t *testing.T) {
	// Claude: hooks enabled, prompt mode "arg"
	rc := &config.RuntimeConfig{
		PromptMode: "arg",
		Hooks: &config.RuntimeHooksConfig{
			Provider: "claude",
		},
	}

	info := GetStartupFallbackInfo(rc)
	if info.IncludePrimeInBeacon {
		t.Error("Hooks+Prompt should NOT include prime instruction in beacon")
	}
	if info.SendStartupNudge {
		t.Error("Hooks+Prompt should NOT need startup nudge (beacon has it)")
	}
}

func TestGetStartupFallbackInfo_HooksNoPrompt(t *testing.T) {
	// Hypothetical agent: hooks enabled but no prompt support
	rc := &config.RuntimeConfig{
		PromptMode: "none",
		Hooks: &config.RuntimeHooksConfig{
			Provider: "claude",
		},
	}

	info := GetStartupFallbackInfo(rc)
	if info.IncludePrimeInBeacon {
		t.Error("Hooks+NoPrompt should NOT include prime instruction (hooks run it)")
	}
	if !info.SendStartupNudge {
		t.Error("Hooks+NoPrompt should need startup nudge (no prompt to include it)")
	}
	if info.StartupNudgeDelayMs != 0 {
		t.Error("Hooks+NoPrompt should NOT wait (hooks already ran gt prime)")
	}
}

func TestGetStartupFallbackInfo_NoHooksWithPrompt(t *testing.T) {
	// Codex/Cursor: no hooks, but has prompt support
	rc := &config.RuntimeConfig{
		PromptMode: "arg",
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	info := GetStartupFallbackInfo(rc)
	if !info.IncludePrimeInBeacon {
		t.Error("NoHooks+Prompt should include prime instruction in beacon")
	}
	if !info.SendStartupNudge {
		t.Error("NoHooks+Prompt should need startup nudge")
	}
	if info.StartupNudgeDelayMs <= 0 {
		t.Error("NoHooks+Prompt should wait for gt prime to complete")
	}
}

func TestGetStartupFallbackInfo_NoHooksNoPrompt(t *testing.T) {
	// Auggie/AMP: no hooks, no prompt support
	rc := &config.RuntimeConfig{
		PromptMode: "none",
		Hooks: &config.RuntimeHooksConfig{
			Provider: "none",
		},
	}

	info := GetStartupFallbackInfo(rc)
	if !info.IncludePrimeInBeacon {
		t.Error("NoHooks+NoPrompt should include prime instruction")
	}
	if !info.SendStartupNudge {
		t.Error("NoHooks+NoPrompt should need startup nudge")
	}
	if info.StartupNudgeDelayMs <= 0 {
		t.Error("NoHooks+NoPrompt should wait for gt prime to complete")
	}
	if !info.SendBeaconNudge {
		t.Error("NoHooks+NoPrompt should send beacon via nudge (no prompt)")
	}
}

func TestGetStartupFallbackInfo_NilConfig(t *testing.T) {
	// Nil config defaults to Claude (hooks enabled, prompt "arg")
	info := GetStartupFallbackInfo(nil)
	if info.IncludePrimeInBeacon {
		t.Error("Nil config (defaults to Claude) should NOT include prime instruction")
	}
	if info.SendStartupNudge {
		t.Error("Nil config (defaults to Claude) should NOT need startup nudge")
	}
}

func TestStartupNudgeContent(t *testing.T) {
	content := StartupNudgeContent()
	if content == "" {
		t.Error("StartupNudgeContent should return non-empty string")
	}
	if !contains(content, "gt hook") {
		t.Error("StartupNudgeContent should mention gt hook")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfiles_ValidConfig(t *testing.T) {
	// Create a temporary directory with a valid town.json
	tmpDir := t.TempDir()
	townJSON := filepath.Join(tmpDir, "town.json")

	validConfig := `{
  "type": "town",
  "version": 2,
  "name": "test-town",
  "profiles": {
    "anthropic_acctA": {
      "provider": "anthropic",
      "auth_ref": "ANTHROPIC_API_KEY_A",
      "model_main": "claude-sonnet-4-20250514",
      "model_fast": "claude-haiku-3-5-20241022"
    },
    "openai_acctA": {
      "provider": "openai",
      "auth_ref": "OPENAI_API_KEY",
      "model_main": "gpt-4o",
      "model_fast": "gpt-4o-mini"
    }
  }
}`

	if err := os.WriteFile(townJSON, []byte(validConfig), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Load profiles
	profiles, err := LoadProfiles(townJSON)
	if err != nil {
		t.Fatalf("LoadProfiles failed: %v", err)
	}

	// Verify we got the expected profiles
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}

	// Check anthropic profile
	anthro, ok := profiles["anthropic_acctA"]
	if !ok {
		t.Fatal("missing anthropic_acctA profile")
	}
	if anthro.Provider != "anthropic" {
		t.Errorf("expected provider 'anthropic', got '%s'", anthro.Provider)
	}
	if anthro.AuthRef != "ANTHROPIC_API_KEY_A" {
		t.Errorf("expected auth_ref 'ANTHROPIC_API_KEY_A', got '%s'", anthro.AuthRef)
	}
	if anthro.ModelMain != "claude-sonnet-4-20250514" {
		t.Errorf("expected model_main 'claude-sonnet-4-20250514', got '%s'", anthro.ModelMain)
	}
	if anthro.ModelFast != "claude-haiku-3-5-20241022" {
		t.Errorf("expected model_fast 'claude-haiku-3-5-20241022', got '%s'", anthro.ModelFast)
	}

	// Check openai profile
	openai, ok := profiles["openai_acctA"]
	if !ok {
		t.Fatal("missing openai_acctA profile")
	}
	if openai.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", openai.Provider)
	}
	if openai.AuthRef != "OPENAI_API_KEY" {
		t.Errorf("expected auth_ref 'OPENAI_API_KEY', got '%s'", openai.AuthRef)
	}
	if openai.ModelMain != "gpt-4o" {
		t.Errorf("expected model_main 'gpt-4o', got '%s'", openai.ModelMain)
	}
	if openai.ModelFast != "gpt-4o-mini" {
		t.Errorf("expected model_fast 'gpt-4o-mini', got '%s'", openai.ModelFast)
	}
}

func TestLoadProfiles_FileNotFound(t *testing.T) {
	_, err := LoadProfiles("/nonexistent/path/town.json")
	if err == nil {
		t.Error("expected error for nonexistent file, got nil")
	}
}

func TestLoadProfiles_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	townJSON := filepath.Join(tmpDir, "town.json")

	if err := os.WriteFile(townJSON, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := LoadProfiles(townJSON)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestLoadProfiles_NoProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	townJSON := filepath.Join(tmpDir, "town.json")

	// Config without profiles field
	configNoProfiles := `{
  "type": "town",
  "version": 2,
  "name": "test-town"
}`

	if err := os.WriteFile(townJSON, []byte(configNoProfiles), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	profiles, err := LoadProfiles(townJSON)
	if err != nil {
		t.Fatalf("LoadProfiles failed: %v", err)
	}

	// Should return empty map, not error
	if profiles == nil {
		t.Error("expected non-nil map, got nil")
	}
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

func TestLoadProfiles_EmptyProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	townJSON := filepath.Join(tmpDir, "town.json")

	configEmptyProfiles := `{
  "type": "town",
  "version": 2,
  "name": "test-town",
  "profiles": {}
}`

	if err := os.WriteFile(townJSON, []byte(configEmptyProfiles), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	profiles, err := LoadProfiles(townJSON)
	if err != nil {
		t.Fatalf("LoadProfiles failed: %v", err)
	}

	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(profiles))
	}
}

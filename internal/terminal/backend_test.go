package terminal

import (
	"testing"
)

// --- parseCoopConfig Tests ---

func TestParseCoopConfig_Empty(t *testing.T) {
	cfg, err := parseCoopConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config for empty input")
	}
}

func TestParseCoopConfig_NoCoop(t *testing.T) {
	cfg, err := parseCoopConfig("backend: k8s\nsome_key: something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Error("expected nil config when 'coop' not present")
	}
}

func TestParseCoopConfig_FullConfig(t *testing.T) {
	input := `backend: coop
coop_url: http://localhost:8080
coop_token: secret-tok`

	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://localhost:8080")
	}
	if cfg.Token != "secret-tok" {
		t.Errorf("Token = %q, want %q", cfg.Token, "secret-tok")
	}
}

func TestParseCoopConfig_MissingURL(t *testing.T) {
	input := `backend: coop
coop_token: secret-tok`

	_, err := parseCoopConfig(input)
	if err == nil {
		t.Fatal("expected error for missing coop_url")
	}
}

func TestParseCoopConfig_NoToken(t *testing.T) {
	input := `backend: coop
coop_url: http://10.0.0.5:8080`

	cfg, err := parseCoopConfig(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.baseURL != "http://10.0.0.5:8080" {
		t.Errorf("baseURL = %q, want %q", cfg.baseURL, "http://10.0.0.5:8080")
	}
	if cfg.Token != "" {
		t.Errorf("Token = %q, want empty", cfg.Token)
	}
}

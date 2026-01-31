package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadBuiltinRoleDefinition(t *testing.T) {
	tests := []struct {
		name          string
		role          string
		wantScope     string
		wantPattern   string
		wantPreSync   bool
	}{
		{
			name:          "mayor",
			role:          "mayor",
			wantScope:     "town",
			wantPattern:   "hq-mayor",
			wantPreSync:   false,
		},
		{
			name:          "deacon",
			role:          "deacon",
			wantScope:     "town",
			wantPattern:   "hq-deacon",
			wantPreSync:   false,
		},
		{
			name:          "witness",
			role:          "witness",
			wantScope:     "rig",
			wantPattern:   "gt-{rig}-witness",
			wantPreSync:   false,
		},
		{
			name:          "refinery",
			role:          "refinery",
			wantScope:     "rig",
			wantPattern:   "gt-{rig}-refinery",
			wantPreSync:   true,
		},
		{
			name:          "polecat",
			role:          "polecat",
			wantScope:     "rig",
			wantPattern:   "gt-{rig}-{name}",
			wantPreSync:   true,
		},
		{
			name:          "crew",
			role:          "crew",
			wantScope:     "rig",
			wantPattern:   "gt-{rig}-crew-{name}",
			wantPreSync:   true,
		},
		{
			name:          "dog",
			role:          "dog",
			wantScope:     "town",
			wantPattern:   "gt-dog-{name}",
			wantPreSync:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, err := loadBuiltinRoleDefinition(tt.role)
			if err != nil {
				t.Fatalf("loadBuiltinRoleDefinition(%s) error: %v", tt.role, err)
			}

			if def.Role != tt.role {
				t.Errorf("Role = %q, want %q", def.Role, tt.role)
			}
			if def.Scope != tt.wantScope {
				t.Errorf("Scope = %q, want %q", def.Scope, tt.wantScope)
			}
			if def.Session.Pattern != tt.wantPattern {
				t.Errorf("Session.Pattern = %q, want %q", def.Session.Pattern, tt.wantPattern)
			}
			if def.Session.NeedsPreSync != tt.wantPreSync {
				t.Errorf("Session.NeedsPreSync = %v, want %v", def.Session.NeedsPreSync, tt.wantPreSync)
			}

			// Verify health config has reasonable defaults
			if def.Health.PingTimeout.Duration == 0 {
				t.Error("Health.PingTimeout should not be zero")
			}
			if def.Health.ConsecutiveFailures == 0 {
				t.Error("Health.ConsecutiveFailures should not be zero")
			}
		})
	}
}

func TestLoadBuiltinRoleDefinition_UnknownRole(t *testing.T) {
	_, err := loadBuiltinRoleDefinition("nonexistent")
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
}

func TestLoadRoleDefinition_UnknownRole(t *testing.T) {
	_, err := LoadRoleDefinition("/tmp/town", "", "nonexistent")
	if err == nil {
		t.Error("expected error for unknown role, got nil")
	}
	// Should have a clear error message, not a cryptic embed error
	if !strings.Contains(err.Error(), "unknown role") {
		t.Errorf("error should mention 'unknown role', got: %v", err)
	}
}

func TestAllRoles(t *testing.T) {
	roles := AllRoles()
	if len(roles) != 7 {
		t.Errorf("AllRoles() returned %d roles, want 7", len(roles))
	}

	expected := map[string]bool{
		"mayor":    true,
		"deacon":   true,
		"dog":      true,
		"witness":  true,
		"refinery": true,
		"polecat":  true,
		"crew":     true,
	}

	for _, r := range roles {
		if !expected[r] {
			t.Errorf("unexpected role %q in AllRoles()", r)
		}
	}
}

func TestTownRoles(t *testing.T) {
	roles := TownRoles()
	if len(roles) != 3 {
		t.Errorf("TownRoles() returned %d roles, want 3", len(roles))
	}

	for _, r := range roles {
		def, err := loadBuiltinRoleDefinition(r)
		if err != nil {
			t.Fatalf("loadBuiltinRoleDefinition(%s) error: %v", r, err)
		}
		if def.Scope != "town" {
			t.Errorf("role %s has scope %q, expected 'town'", r, def.Scope)
		}
	}
}

func TestRigRoles(t *testing.T) {
	roles := RigRoles()
	if len(roles) != 4 {
		t.Errorf("RigRoles() returned %d roles, want 4", len(roles))
	}

	for _, r := range roles {
		def, err := loadBuiltinRoleDefinition(r)
		if err != nil {
			t.Fatalf("loadBuiltinRoleDefinition(%s) error: %v", r, err)
		}
		if def.Scope != "rig" {
			t.Errorf("role %s has scope %q, expected 'rig'", r, def.Scope)
		}
	}
}

func TestExpandPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		town     string
		rig      string
		name     string
		role     string
		expected string
	}{
		{
			pattern:  "{town}",
			town:     "/home/user/gt",
			expected: "/home/user/gt",
		},
		{
			pattern:  "gt-{rig}-witness",
			rig:      "gastown",
			expected: "gt-gastown-witness",
		},
		{
			pattern:  "{town}/{rig}/crew/{name}",
			town:     "/home/user/gt",
			rig:      "gastown",
			name:     "max",
			expected: "/home/user/gt/gastown/crew/max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := ExpandPattern(tt.pattern, tt.town, tt.rig, tt.name, tt.role)
			if got != tt.expected {
				t.Errorf("ExpandPattern() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestDuration_UnmarshalText(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"30s", 30 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h", time.Hour},
		{"1h30m", time.Hour + 30*time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			if err := d.UnmarshalText([]byte(tt.input)); err != nil {
				t.Fatalf("UnmarshalText() error: %v", err)
			}
			if d.Duration != tt.expected {
				t.Errorf("Duration = %v, want %v", d.Duration, tt.expected)
			}
		})
	}
}

func TestToLegacyRoleConfig(t *testing.T) {
	def := &RoleDefinition{
		Role:  "witness",
		Scope: "rig",
		Session: RoleSessionConfig{
			Pattern:      "gt-{rig}-witness",
			WorkDir:      "{town}/{rig}/witness",
			NeedsPreSync: false,
			StartCommand: "exec claude",
		},
		Env: map[string]string{"GT_ROLE": "witness"},
		Health: RoleHealthConfig{
			PingTimeout:         Duration{30 * time.Second},
			ConsecutiveFailures: 3,
			KillCooldown:        Duration{5 * time.Minute},
			StuckThreshold:      Duration{time.Hour},
		},
	}

	legacy := def.ToLegacyRoleConfig()

	if legacy.SessionPattern != "gt-{rig}-witness" {
		t.Errorf("SessionPattern = %q, want %q", legacy.SessionPattern, "gt-{rig}-witness")
	}
	if legacy.WorkDirPattern != "{town}/{rig}/witness" {
		t.Errorf("WorkDirPattern = %q, want %q", legacy.WorkDirPattern, "{town}/{rig}/witness")
	}
	if legacy.NeedsPreSync != false {
		t.Errorf("NeedsPreSync = %v, want false", legacy.NeedsPreSync)
	}
	if legacy.PingTimeout != "30s" {
		t.Errorf("PingTimeout = %q, want %q", legacy.PingTimeout, "30s")
	}
	if legacy.ConsecutiveFailures != 3 {
		t.Errorf("ConsecutiveFailures = %d, want 3", legacy.ConsecutiveFailures)
	}
}

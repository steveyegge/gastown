package conductor

import (
	"testing"
)

func TestAllPhases_Count(t *testing.T) {
	phases := AllPhases()
	if len(phases) != 7 {
		t.Fatalf("AllPhases() returned %d phases, want 7", len(phases))
	}
}

func TestAllPhases_Order(t *testing.T) {
	phases := AllPhases()
	expected := []string{"examine", "harden", "modernize", "specify", "implement", "secure", "document"}
	for i, name := range expected {
		if phases[i].Name != name {
			t.Errorf("AllPhases()[%d].Name = %q, want %q", i, phases[i].Name, name)
		}
	}
}

func TestAllPhases_PhaseNumbers(t *testing.T) {
	phases := AllPhases()
	for i, p := range phases {
		if int(p.Phase) != i+1 {
			t.Errorf("AllPhases()[%d].Phase = %d, want %d", i, p.Phase, i+1)
		}
	}
}

func TestAllPhases_HaveDescriptions(t *testing.T) {
	for _, p := range AllPhases() {
		if p.Description == "" {
			t.Errorf("phase %q has empty description", p.Name)
		}
	}
}

func TestAllPhases_HaveGates(t *testing.T) {
	for _, p := range AllPhases() {
		if p.Gate == "" {
			t.Errorf("phase %q has empty gate", p.Name)
		}
	}
}

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseExamine, "examine"},
		{PhaseHarden, "harden"},
		{PhaseModernize, "modernize"},
		{PhaseSpecify, "specify"},
		{PhaseImplement, "implement"},
		{PhaseSecure, "secure"},
		{PhaseDocument, "document"},
		{Phase(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.phase.String(); got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestGetPhaseInfo(t *testing.T) {
	info := GetPhaseInfo(PhaseSpecify)
	if info == nil {
		t.Fatal("GetPhaseInfo(PhaseSpecify) returned nil")
	}
	if info.Name != "specify" {
		t.Errorf("Name = %q, want %q", info.Name, "specify")
	}
	if !info.UserGate {
		t.Error("specify phase should have UserGate=true")
	}
}

func TestGetPhaseInfo_NotFound(t *testing.T) {
	info := GetPhaseInfo(Phase(99))
	if info != nil {
		t.Error("GetPhaseInfo(99) should return nil")
	}
}

func TestPhaseByName(t *testing.T) {
	p, ok := PhaseByName("implement")
	if !ok {
		t.Fatal("PhaseByName(\"implement\") returned false")
	}
	if p != PhaseImplement {
		t.Errorf("PhaseByName(\"implement\") = %d, want %d", p, PhaseImplement)
	}
}

func TestPhaseByName_NotFound(t *testing.T) {
	_, ok := PhaseByName("nonexistent")
	if ok {
		t.Error("PhaseByName(\"nonexistent\") should return false")
	}
}

func TestSpecifyIsOnlyUserGate(t *testing.T) {
	for _, p := range AllPhases() {
		if p.UserGate && p.Phase != PhaseSpecify {
			t.Errorf("phase %q has UserGate=true, expected only specify", p.Name)
		}
	}
}

func TestPhaseOwners(t *testing.T) {
	tests := []struct {
		phase Phase
		owner string
	}{
		{PhaseExamine, "architect"},
		{PhaseHarden, "tests"},
		{PhaseSpecify, "tests"},
		{PhaseSecure, "security"},
		{PhaseDocument, "docs"},
	}
	for _, tt := range tests {
		info := GetPhaseInfo(tt.phase)
		if info.Owner != tt.owner {
			t.Errorf("phase %s owner = %q, want %q", info.Name, info.Owner, tt.owner)
		}
	}
}

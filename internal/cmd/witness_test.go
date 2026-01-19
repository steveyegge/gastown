package cmd

import (
	"strings"
	"testing"
)

// --- Env Override Parsing Tests ---

func TestSplitEnvOverride_ValidKeyValue(t *testing.T) {
	parts := splitEnvOverride("KEY=value")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "KEY" {
		t.Errorf("expected key 'KEY', got %q", parts[0])
	}
	if parts[1] != "value" {
		t.Errorf("expected value 'value', got %q", parts[1])
	}
}

func TestSplitEnvOverride_ValueContainsEquals(t *testing.T) {
	parts := splitEnvOverride("KEY=value=with=equals")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "KEY" {
		t.Errorf("expected key 'KEY', got %q", parts[0])
	}
	if parts[1] != "value=with=equals" {
		t.Errorf("expected value 'value=with=equals', got %q", parts[1])
	}
}

func TestSplitEnvOverride_EmptyValue(t *testing.T) {
	parts := splitEnvOverride("KEY=")
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(parts))
	}
	if parts[0] != "KEY" {
		t.Errorf("expected key 'KEY', got %q", parts[0])
	}
	if parts[1] != "" {
		t.Errorf("expected empty value, got %q", parts[1])
	}
}

func TestSplitEnvOverride_NoEquals_ReturnsNil(t *testing.T) {
	parts := splitEnvOverride("KEYONLY")
	if parts != nil {
		t.Errorf("expected nil for invalid format, got %v", parts)
	}
}

func TestParseEnvOverrides_MultipleValidEntries(t *testing.T) {
	overrides := []string{"KEY1=value1", "KEY2=value2", "KEY3=value3"}
	result := parseEnvOverrides(overrides)

	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result))
	}
	if result["KEY1"] != "value1" {
		t.Errorf("KEY1: expected 'value1', got %q", result["KEY1"])
	}
	if result["KEY2"] != "value2" {
		t.Errorf("KEY2: expected 'value2', got %q", result["KEY2"])
	}
	if result["KEY3"] != "value3" {
		t.Errorf("KEY3: expected 'value3', got %q", result["KEY3"])
	}
}

func TestParseEnvOverrides_SkipsInvalidEntries(t *testing.T) {
	overrides := []string{"VALID=ok", "INVALID", "ALSO_VALID=yes"}
	result := parseEnvOverrides(overrides)

	if len(result) != 2 {
		t.Fatalf("expected 2 valid entries, got %d", len(result))
	}
	if result["VALID"] != "ok" {
		t.Errorf("VALID: expected 'ok', got %q", result["VALID"])
	}
	if result["ALSO_VALID"] != "yes" {
		t.Errorf("ALSO_VALID: expected 'yes', got %q", result["ALSO_VALID"])
	}
}

func TestParseEnvOverrides_EmptySlice_ReturnsEmptyMap(t *testing.T) {
	result := parseEnvOverrides([]string{})
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestParseEnvOverrides_LastValueWins(t *testing.T) {
	overrides := []string{"KEY=first", "KEY=second", "KEY=third"}
	result := parseEnvOverrides(overrides)

	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result["KEY"] != "third" {
		t.Errorf("expected last value 'third', got %q", result["KEY"])
	}
}

// --- Flag Definition Tests ---

func TestWitnessRestartAgentFlag(t *testing.T) {
	flag := witnessRestartCmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("expected witness restart to define --agent flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default agent override to be empty, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "overrides town default") {
		t.Errorf("expected --agent usage to mention overrides town default, got %q", flag.Usage)
	}
}

func TestWitnessStartAgentFlag(t *testing.T) {
	flag := witnessStartCmd.Flags().Lookup("agent")
	if flag == nil {
		t.Fatal("expected witness start to define --agent flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default agent override to be empty, got %q", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "overrides town default") {
		t.Errorf("expected --agent usage to mention overrides town default, got %q", flag.Usage)
	}
}

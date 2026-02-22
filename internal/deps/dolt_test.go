package deps

import "testing"

func TestParseDoltVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"dolt version 1.82.4", "1.82.4"},
		{"dolt version 1.82.4\n", "1.82.4"},
		{"dolt version 1.0.0", "1.0.0"},
		{"dolt version 10.20.30", "10.20.30"},
		{"some other output", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := parseDoltVersion(tt.input)
		if result != tt.expected {
			t.Errorf("parseDoltVersion(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCheckDolt(t *testing.T) {
	status, version, _ := CheckDolt()

	if status == DoltNotFound {
		t.Skip("dolt not installed, skipping integration test")
	}

	if status == DoltOK && version == "" {
		t.Error("CheckDolt returned DoltOK but empty version")
	}

	t.Logf("CheckDolt: status=%d, version=%s", status, version)
}

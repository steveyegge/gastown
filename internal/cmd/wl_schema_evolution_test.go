package cmd

import (
	"testing"
)

func TestParseSchemaVersion_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input      string
		wantMajor  int
		wantMinor  int
	}{
		{"1.0", 1, 0},
		{"2.3", 2, 3},
		{"0.0", 0, 0},
		{"10.42", 10, 42},
		{" 1.0 ", 1, 0}, // leading/trailing space
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			major, minor, err := ParseSchemaVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseSchemaVersion(%q) unexpected error: %v", tt.input, err)
			}
			if major != tt.wantMajor || minor != tt.wantMinor {
				t.Errorf("ParseSchemaVersion(%q) = (%d, %d), want (%d, %d)",
					tt.input, major, minor, tt.wantMajor, tt.wantMinor)
			}
		})
	}
}

func TestParseSchemaVersion_Invalid(t *testing.T) {
	t.Parallel()
	cases := []string{"1", "1.0.0", "", "abc", "1.x", "x.0"}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			_, _, err := ParseSchemaVersion(input)
			if err == nil {
				t.Errorf("ParseSchemaVersion(%q) expected error, got nil", input)
			}
		})
	}
}

func TestClassifySchemaChange(t *testing.T) {
	t.Parallel()
	tests := []struct {
		local    string
		upstream string
		want     SchemaChangeKind
	}{
		// Unchanged
		{"1.0", "1.0", SchemaUnchanged},
		{"2.5", "2.5", SchemaUnchanged},
		// Minor bump
		{"1.0", "1.1", SchemaMinorChange},
		{"1.0", "1.9", SchemaMinorChange},
		{"2.0", "2.1", SchemaMinorChange},
		// Major bump
		{"1.0", "2.0", SchemaMajorChange},
		{"1.5", "2.0", SchemaMajorChange},
		{"1.9", "3.0", SchemaMajorChange},
		// Downgrade (local newer) — treated as unchanged
		{"1.1", "1.0", SchemaUnchanged},
		{"2.0", "1.9", SchemaUnchanged},
	}
	for _, tt := range tests {
		t.Run(tt.local+"->"+tt.upstream, func(t *testing.T) {
			t.Parallel()
			got, err := ClassifySchemaChange(tt.local, tt.upstream)
			if err != nil {
				t.Fatalf("ClassifySchemaChange(%q, %q) unexpected error: %v", tt.local, tt.upstream, err)
			}
			if got != tt.want {
				t.Errorf("ClassifySchemaChange(%q, %q) = %v, want %v",
					tt.local, tt.upstream, got, tt.want)
			}
		})
	}
}

func TestClassifySchemaChange_InvalidVersion(t *testing.T) {
	t.Parallel()
	_, err := ClassifySchemaChange("bad", "1.0")
	if err == nil {
		t.Error("expected error for invalid local version")
	}
	_, err = ClassifySchemaChange("1.0", "bad")
	if err == nil {
		t.Error("expected error for invalid upstream version")
	}
}

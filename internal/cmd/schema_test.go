package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSchemaDetail(t *testing.T) {
	detail, err := buildSchemaDetail(rootCmd, []string{"version"})
	if err != nil {
		t.Fatalf("buildSchemaDetail(version): %v", err)
	}

	if detail.Name != "gt version" {
		t.Fatalf("detail.Name = %q, want %q", detail.Name, "gt version")
	}

	if detail.Schema != jsonSchemaDraft {
		t.Fatalf("detail.Schema = %q, want %q", detail.Schema, jsonSchemaDraft)
	}

	// version command has --verbose and --short flags
	if detail.Flags == nil {
		t.Fatal("detail.Flags is nil")
	}

	props, ok := detail.Flags["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("detail.Flags has no properties")
	}

	if _, ok := props["verbose"]; !ok {
		t.Error("expected 'verbose' flag in schema")
	}
}

func TestSchemaDetailSubcommand(t *testing.T) {
	_, err := buildSchemaDetail(rootCmd, []string{"schema"})
	if err != nil {
		t.Fatalf("buildSchemaDetail(schema): %v", err)
	}
}

func TestSchemaDetailUnknownCommand(t *testing.T) {
	_, err := buildSchemaDetail(rootCmd, []string{"nonexistent-command-xyz"})
	if err == nil {
		t.Fatal("expected error for unknown command, got nil")
	}
}

func TestFlagTypeToJSONSchema(t *testing.T) {
	tests := []struct {
		flagType string
		want     string
	}{
		{"bool", "boolean"},
		{"string", "string"},
		{"int", "integer"},
		{"int32", "integer"},
		{"int64", "integer"},
		{"float32", "number"},
		{"float64", "number"},
		{"stringSlice", "array"},
		{"stringArray", "array"},
		{"unknown", "string"},
	}

	for _, tt := range tests {
		t.Run(tt.flagType, func(t *testing.T) {
			got := flagTypeToJSONSchema(tt.flagType)
			if got != tt.want {
				t.Errorf("flagTypeToJSONSchema(%q) = %q, want %q", tt.flagType, got, tt.want)
			}
		})
	}
}

func TestSchemaIndex(t *testing.T) {
	index := buildSchemaIndex(rootCmd)

	if index.Name != "gt" {
		t.Fatalf("index.Name = %q, want %q", index.Name, "gt")
	}

	if len(index.Commands) == 0 {
		t.Fatal("index.Commands is empty, want at least one command")
	}

	// Verify a known command is present
	found := false
	for _, c := range index.Commands {
		if c.Name == "gt version" {
			found = true
			if c.Description == "" {
				t.Error("version command has empty description")
			}
			break
		}
	}
	if !found {
		t.Fatal("expected to find 'gt version' in index")
	}

	// Verify JSON marshaling works
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("marshaled JSON is empty")
	}
}

func TestSchemaDetailIncludesOutputSchema(t *testing.T) {
	// Create a test command with an output schema annotation
	testCmd := &cobra.Command{
		Use:   "test-enriched",
		Short: "Test command with output schema",
		Annotations: map[string]string{
			AnnotationOutputSchema: `{"type":"object","properties":{"ok":{"type":"boolean"}}}`,
		},
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	rootCmd.AddCommand(testCmd)
	t.Cleanup(func() {
		rootCmd.RemoveCommand(testCmd)
	})

	detail, err := buildSchemaDetail(rootCmd, []string{"test-enriched"})
	if err != nil {
		t.Fatalf("buildSchemaDetail: %v", err)
	}

	if detail.OutputSchema == nil {
		t.Fatal("expected outputSchema to be set from annotation")
	}

	// Verify it round-trips as valid JSON
	data, err := json.Marshal(detail.OutputSchema)
	if err != nil {
		t.Fatalf("outputSchema marshal failed: %v", err)
	}
	if !strings.Contains(string(data), `"boolean"`) {
		t.Errorf("outputSchema missing expected content, got: %s", data)
	}
}

func TestSchemaIndexHiddenCommandsIncluded(t *testing.T) {
	index := buildSchemaIndex(rootCmd)

	// Hidden commands should be included (with hidden:true flag)
	// so harnesses can discover internal commands if needed
	hasHidden := false
	for _, c := range index.Commands {
		if c.Hidden {
			hasHidden = true
			break
		}
	}
	// proxy-subcmds is hidden, so this should be true
	if !hasHidden {
		t.Error("expected at least one hidden command in index")
	}
}

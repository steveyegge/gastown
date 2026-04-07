package cmd

import (
	"encoding/json"
	"testing"
)

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

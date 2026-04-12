package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrackingDependsOnID_CrossRigWrapsExternal(t *testing.T) {
	townRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(townRoot, ".beads"), 0o755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, ".beads", "routes.jsonl"), []byte("{\"prefix\":\"ag-\",\"path\":\"agentcompany/.beads\"}\n"), 0o644); err != nil {
		t.Fatalf("write routes.jsonl: %v", err)
	}

	got := trackingDependsOnID(townRoot, "ag-95s.1")
	want := "external:ag:ag-95s.1"
	if got != want {
		t.Fatalf("trackingDependsOnID() = %q, want %q", got, want)
	}
}

func TestTrackingDependsOnID_HQStaysLocal(t *testing.T) {
	townRoot := t.TempDir()
	got := trackingDependsOnID(townRoot, "hq-cv-test")
	if got != "hq-cv-test" {
		t.Fatalf("trackingDependsOnID() = %q, want %q", got, "hq-cv-test")
	}
}

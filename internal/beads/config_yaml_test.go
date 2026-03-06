package beads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureConfigYAMLIfMissing_DoesNotOverwriteExisting(t *testing.T) {
	beadsDir := t.TempDir()
	configPath := filepath.Join(beadsDir, "config.yaml")
	original := "prefix: keep\nissue-prefix: keep\n"
	if err := os.WriteFile(configPath, []byte(original), 0644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	if err := EnsureConfigYAMLIfMissing(beadsDir, "hq"); err != nil {
		t.Fatalf("EnsureConfigYAMLIfMissing: %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if string(after) != original {
		t.Fatalf("config.yaml changed:\n got: %q\nwant: %q", string(after), original)
	}
}

func TestEnsureConfigYAMLFromMetadataIfMissing_UsesMetadataPrefix(t *testing.T) {
	beadsDir := t.TempDir()
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"hq","issue_prefix":"foo"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	if err := EnsureConfigYAMLFromMetadataIfMissing(beadsDir, "hq"); err != nil {
		t.Fatalf("EnsureConfigYAMLFromMetadataIfMissing: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "prefix: foo\n") {
		t.Fatalf("config.yaml missing metadata prefix: %q", got)
	}
	if !strings.Contains(got, "issue-prefix: foo\n") {
		t.Fatalf("config.yaml missing metadata issue-prefix: %q", got)
	}
}

func TestConfigDefaultsFromMetadata_FallsBackToDoltDatabase(t *testing.T) {
	beadsDir := t.TempDir()
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"hq-custom"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	prefix := ConfigDefaultsFromMetadata(beadsDir, "hq")
	if prefix != "hq-custom" {
		t.Fatalf("prefix = %q, want %q", prefix, "hq-custom")
	}
}

func TestConfigDefaultsFromMetadata_StripsLegacyBeadsPrefixFromDoltDatabase(t *testing.T) {
	beadsDir := t.TempDir()
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"beads_hq"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	prefix := ConfigDefaultsFromMetadata(beadsDir, "fallback")
	if prefix != "hq" {
		t.Fatalf("prefix = %q, want %q", prefix, "hq")
	}
}

func TestEnsureConfigYAMLFromMetadataIfMissing_StripsLegacyBeadsPrefixFromDoltDatabase(t *testing.T) {
	beadsDir := t.TempDir()
	metadata := `{"backend":"dolt","dolt_mode":"server","dolt_database":"beads_hq"}`
	if err := os.WriteFile(filepath.Join(beadsDir, "metadata.json"), []byte(metadata), 0644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	if err := EnsureConfigYAMLFromMetadataIfMissing(beadsDir, "fallback"); err != nil {
		t.Fatalf("EnsureConfigYAMLFromMetadataIfMissing: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(beadsDir, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "prefix: hq\n") {
		t.Fatalf("config.yaml missing normalized prefix: %q", got)
	}
	if !strings.Contains(got, "issue-prefix: hq\n") {
		t.Fatalf("config.yaml missing normalized issue-prefix: %q", got)
	}
}

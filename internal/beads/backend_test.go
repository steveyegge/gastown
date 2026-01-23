package beads_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/beads"
)

// TestBackendDetection verifies that both SQLite and Dolt backends can be detected
// from metadata.json configuration.
func TestBackendDetection(t *testing.T) {
	tests := []struct {
		name            string
		metadata        string
		expectedBackend string
	}{
		{
			name: "SQLite backend (explicit)",
			metadata: `{
				"database": "beads.db",
				"jsonl_export": "issues.jsonl",
				"backend": "sqlite"
			}`,
			expectedBackend: "sqlite",
		},
		{
			name: "Dolt backend",
			metadata: `{
				"jsonl_export": "issues.jsonl",
				"backend": "dolt",
				"prefix": "gt"
			}`,
			expectedBackend: "dolt",
		},
		{
			name: "No backend specified (defaults to SQLite)",
			metadata: `{
				"database": "beads.db",
				"jsonl_export": "issues.jsonl"
			}`,
			expectedBackend: "sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory with metadata.json
			tmpDir := t.TempDir()
			metadataPath := filepath.Join(tmpDir, "metadata.json")

			if err := os.WriteFile(metadataPath, []byte(tt.metadata), 0644); err != nil {
				t.Fatalf("Failed to write metadata.json: %v", err)
			}

			got := beads.GetStorageBackend(tmpDir)
			if got != tt.expectedBackend {
				t.Errorf("GetStorageBackend() = %q, want %q", got, tt.expectedBackend)
			}
		})
	}
}

// TestGetStorageBackend_ConfigYAML tests backend detection from config.yaml
func TestGetStorageBackend_ConfigYAML(t *testing.T) {
	tests := []struct {
		name            string
		configYAML      string
		expectedBackend string
	}{
		{
			name:            "Dolt backend from config.yaml",
			configYAML:      "prefix: hq\nstorage-backend: dolt\n",
			expectedBackend: "dolt",
		},
		{
			name:            "SQLite backend from config.yaml",
			configYAML:      "prefix: hq\nstorage-backend: sqlite\n",
			expectedBackend: "sqlite",
		},
		{
			name:            "No backend in config.yaml defaults to sqlite",
			configYAML:      "prefix: hq\n",
			expectedBackend: "sqlite",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")

			if err := os.WriteFile(configPath, []byte(tt.configYAML), 0644); err != nil {
				t.Fatalf("Failed to write config.yaml: %v", err)
			}

			got := beads.GetStorageBackend(tmpDir)
			if got != tt.expectedBackend {
				t.Errorf("GetStorageBackend() = %q, want %q", got, tt.expectedBackend)
			}
		})
	}
}

// TestGetStorageBackend_MetadataOverridesConfig tests that metadata.json takes precedence
func TestGetStorageBackend_MetadataOverridesConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Write config.yaml with sqlite
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("storage-backend: sqlite\n"), 0644); err != nil {
		t.Fatalf("Failed to write config.yaml: %v", err)
	}

	// Write metadata.json with dolt - should take precedence
	metadataPath := filepath.Join(tmpDir, "metadata.json")
	if err := os.WriteFile(metadataPath, []byte(`{"backend": "dolt"}`), 0644); err != nil {
		t.Fatalf("Failed to write metadata.json: %v", err)
	}

	got := beads.GetStorageBackend(tmpDir)
	if got != "dolt" {
		t.Errorf("GetStorageBackend() = %q, want %q (metadata.json should override config.yaml)", got, "dolt")
	}
}

// TestCreateIssue_BothBackends is a placeholder for testing issue creation
// against both SQLite and Dolt backends.
func TestCreateIssue_BothBackends(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping backend integration test in short mode")
	}

	backends := []string{"sqlite", "dolt"}

	for _, backend := range backends {
		backend := backend // capture range variable
		t.Run("Backend_"+backend, func(t *testing.T) {
			// TODO: Setup test database for backend
			// TODO: Create test issue
			// TODO: Verify issue was created
			// TODO: Test list/show/update/close operations
			t.Logf("TODO: Implement %s backend tests", backend)
		})
	}
}

// TestListIssues_BothBackends is a placeholder for testing issue listing
// against both SQLite and Dolt backends.
func TestListIssues_BothBackends(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping backend integration test in short mode")
	}

	backends := []string{"sqlite", "dolt"}

	for _, backend := range backends {
		backend := backend // capture range variable
		t.Run("Backend_"+backend, func(t *testing.T) {
			// TODO: Setup test database with sample issues
			// TODO: Test various list filters (status, priority, type)
			// TODO: Verify consistent results across backends
			t.Logf("TODO: Implement %s backend list tests", backend)
		})
	}
}

// TestDependencies_BothBackends is a placeholder for testing dependency tracking
// against both SQLite and Dolt backends.
func TestDependencies_BothBackends(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping backend integration test in short mode")
	}

	backends := []string{"sqlite", "dolt"}

	for _, backend := range backends {
		backend := backend // capture range variable
		t.Run("Backend_"+backend, func(t *testing.T) {
			// TODO: Create issues with dependencies
			// TODO: Test "tracks" type dependencies (convoy tracking)
			// TODO: Verify dep list works correctly
			t.Logf("TODO: Implement %s backend dependency tests", backend)
		})
	}
}

// TestConcurrentAccess_Dolt tests Dolt-specific lock contention behavior.
// Dolt uses Git-style lock files and should handle concurrent access gracefully.
func TestConcurrentAccess_Dolt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Dolt-specific test in short mode")
	}

	t.Run("Lock_contention", func(t *testing.T) {
		// TODO: Setup Dolt backend
		// TODO: Simulate concurrent write attempts
		// TODO: Verify one succeeds and others get readonly errors or retry
		t.Log("TODO: Implement Dolt lock contention tests")
	})

	t.Run("Readonly_fallback", func(t *testing.T) {
		// TODO: Test that readonly mode is detected correctly
		// TODO: Verify appropriate error messages
		t.Log("TODO: Implement Dolt readonly fallback tests")
	})
}

// TestBackendSwitching tests migration between SQLite and Dolt backends.
func TestBackendSwitching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping backend switching test in short mode")
	}

	t.Run("SQLite_to_Dolt", func(t *testing.T) {
		// TODO: Create SQLite database with sample data
		// TODO: Export to JSONL
		// TODO: Initialize Dolt backend
		// TODO: Import from JSONL
		// TODO: Verify data integrity
		t.Log("TODO: Implement SQLite to Dolt migration tests")
	})

	t.Run("Config_changes", func(t *testing.T) {
		// TODO: Verify metadata.json changes
		// TODO: Verify config.yaml changes (no-auto-import for Dolt)
		t.Log("TODO: Implement backend config change tests")
	})
}

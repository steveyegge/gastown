package rig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestDetectTechStack(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected TechStack
	}{
		{
			name:     "Go project",
			files:    []string{"go.mod"},
			expected: TechStackGo,
		},
		{
			name:     "Node project",
			files:    []string{"package.json"},
			expected: TechStackNode,
		},
		{
			name:     "Rust project",
			files:    []string{"Cargo.toml"},
			expected: TechStackRust,
		},
		{
			name:     "Python project with pyproject.toml",
			files:    []string{"pyproject.toml"},
			expected: TechStackPython,
		},
		{
			name:     "Python project with setup.py",
			files:    []string{"setup.py"},
			expected: TechStackPython,
		},
		{
			name:     "Python project with requirements.txt",
			files:    []string{"requirements.txt"},
			expected: TechStackPython,
		},
		{
			name:     "Ruby project",
			files:    []string{"Gemfile"},
			expected: TechStackRuby,
		},
		{
			name:     "Unknown project",
			files:    []string{"README.md"},
			expected: TechStackUnknown,
		},
		{
			name:     "Empty project",
			files:    []string{},
			expected: TechStackUnknown,
		},
		{
			name:     "Go takes precedence over Node",
			files:    []string{"go.mod", "package.json"},
			expected: TechStackGo,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create marker files (with directories if needed)
			for _, f := range tc.files {
				path := filepath.Join(tmpDir, f)
				dir := filepath.Dir(path)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create dir %s: %v", dir, err)
				}
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("failed to create %s: %v", f, err)
				}
			}

			result := detectTechStack(tmpDir)
			if result != tc.expected {
				t.Errorf("detectTechStack() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestStackCommands(t *testing.T) {
	// Verify all non-unknown stacks have commands defined
	stacks := []TechStack{TechStackGo, TechStackNode, TechStackRust, TechStackPython, TechStackRuby}

	for _, stack := range stacks {
		t.Run(string(stack), func(t *testing.T) {
			cmds, ok := stackCommands[stack]
			if !ok {
				t.Errorf("stackCommands missing entry for %q", stack)
				return
			}
			if cmds.Test == "" {
				t.Errorf("stackCommands[%q].Test is empty", stack)
			}
			if cmds.Lint == "" {
				t.Errorf("stackCommands[%q].Lint is empty", stack)
			}
			if cmds.Build == "" {
				t.Errorf("stackCommands[%q].Build is empty", stack)
			}
		})
	}
}

func TestGenerateRigSettings(t *testing.T) {
	tests := []struct {
		name          string
		files         []string
		expectFile    bool
		expectedTest  string
		expectedLint  string
		expectedBuild string
	}{
		{
			name:          "Go project generates settings",
			files:         []string{"go.mod"},
			expectFile:    true,
			expectedTest:  "go test ./...",
			expectedLint:  "golangci-lint run ./...",
			expectedBuild: "go build ./...",
		},
		{
			name:          "Node project generates settings",
			files:         []string{"package.json"},
			expectFile:    true,
			expectedTest:  "npm test",
			expectedLint:  "npm run lint",
			expectedBuild: "npm run build",
		},
		{
			name:          "Ruby project generates settings",
			files:         []string{"Gemfile"},
			expectFile:    true,
			expectedTest:  "bundle exec rspec",
			expectedLint:  "bundle exec rubocop",
			expectedBuild: "bundle exec rake build",
		},
		{
			name:       "Unknown project does not generate settings",
			files:      []string{"README.md"},
			expectFile: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rigDir := t.TempDir()
			repoDir := t.TempDir()

			// Create marker files in repo
			for _, f := range tc.files {
				path := filepath.Join(repoDir, f)
				if err := os.WriteFile(path, []byte(""), 0644); err != nil {
					t.Fatalf("failed to create %s: %v", f, err)
				}
			}

			// Create manager (minimal, just for the method)
			m := &Manager{}

			err := m.generateRigSettings(rigDir, repoDir)
			if err != nil {
				t.Fatalf("generateRigSettings() error: %v", err)
			}

			settingsPath := filepath.Join(rigDir, "settings", "config.json")
			_, statErr := os.Stat(settingsPath)

			if tc.expectFile {
				if os.IsNotExist(statErr) {
					t.Fatal("expected settings/config.json to be created, but it wasn't")
				}

				// Read and verify contents
				data, err := os.ReadFile(settingsPath)
				if err != nil {
					t.Fatalf("failed to read settings: %v", err)
				}

				var settings config.RigSettings
				if err := json.Unmarshal(data, &settings); err != nil {
					t.Fatalf("failed to parse settings: %v", err)
				}

				if settings.MergeQueue == nil {
					t.Fatal("settings.MergeQueue is nil")
				}

				if settings.MergeQueue.TestCommand != tc.expectedTest {
					t.Errorf("TestCommand = %q, want %q", settings.MergeQueue.TestCommand, tc.expectedTest)
				}
				if settings.MergeQueue.LintCommand != tc.expectedLint {
					t.Errorf("LintCommand = %q, want %q", settings.MergeQueue.LintCommand, tc.expectedLint)
				}
				if settings.MergeQueue.BuildCommand != tc.expectedBuild {
					t.Errorf("BuildCommand = %q, want %q", settings.MergeQueue.BuildCommand, tc.expectedBuild)
				}
			} else {
				if statErr == nil {
					t.Error("expected no settings file for unknown stack, but one was created")
				}
			}
		})
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Test existing file
	existingFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(existingFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if !fileExists(existingFile) {
		t.Error("fileExists() returned false for existing file")
	}

	// Test non-existing file
	nonExisting := filepath.Join(tmpDir, "does-not-exist.txt")
	if fileExists(nonExisting) {
		t.Error("fileExists() returned true for non-existing file")
	}
}

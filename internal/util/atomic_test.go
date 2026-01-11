package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestAtomicWriteJSON(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")

	// Test basic write
	data := map[string]string{"key": "value"}
	if err := AtomicWriteJSON(testFile, data); err != nil {
		t.Fatalf("AtomicWriteJSON error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("File was not created")
	}

	// Verify temp file was cleaned up
	tmpFile := testFile + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatal("Temp file was not cleaned up")
	}

	// Read and verify content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != "{\n  \"key\": \"value\"\n}" {
		t.Fatalf("Unexpected content: %s", content)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Test basic write
	data := []byte("hello world")
	if err := AtomicWriteFile(testFile, data, 0644); err != nil {
		t.Fatalf("AtomicWriteFile error: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != "hello world" {
		t.Fatalf("Unexpected content: %s", content)
	}

	// Verify temp file was cleaned up
	tmpFile := testFile + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Fatal("Temp file was not cleaned up")
	}
}

func TestAtomicWriteOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")

	// Write initial content
	if err := AtomicWriteJSON(testFile, "first"); err != nil {
		t.Fatalf("First write error: %v", err)
	}

	// Overwrite with new content
	if err := AtomicWriteJSON(testFile, "second"); err != nil {
		t.Fatalf("Second write error: %v", err)
	}

	// Verify new content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != "\"second\"" {
		t.Fatalf("Unexpected content: %s", content)
	}
}

func TestAtomicWriteFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// Test with specific permissions
	data := []byte("test data")
	if err := AtomicWriteFile(testFile, data, 0600); err != nil {
		t.Fatalf("AtomicWriteFile error: %v", err)
	}

	// Verify permissions (on Unix systems)
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	// Check that owner read/write bits are set
	perm := info.Mode().Perm()
	if perm&0600 != 0600 {
		t.Errorf("Expected owner read/write permissions, got %o", perm)
	}
}

func TestAtomicWriteFileEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")

	// Test writing empty data
	if err := AtomicWriteFile(testFile, []byte{}, 0644); err != nil {
		t.Fatalf("AtomicWriteFile error: %v", err)
	}

	// Verify file exists and is empty
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(content) != 0 {
		t.Fatalf("Expected empty file, got %d bytes", len(content))
	}
}

func TestAtomicWriteJSONTypes(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		data     interface{}
		expected string
	}{
		{"string", "hello", `"hello"`},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool", true, "true"},
		{"null", nil, "null"},
		{"array", []int{1, 2, 3}, "[\n  1,\n  2,\n  3\n]"},
		{"nested", map[string]interface{}{"a": map[string]int{"b": 1}}, "{\n  \"a\": {\n    \"b\": 1\n  }\n}"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testFile := filepath.Join(tmpDir, tc.name+".json")
			if err := AtomicWriteJSON(testFile, tc.data); err != nil {
				t.Fatalf("AtomicWriteJSON error: %v", err)
			}

			content, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatalf("ReadFile error: %v", err)
			}
			if string(content) != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, string(content))
			}
		})
	}
}

func TestAtomicWriteJSONUnmarshallable(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unmarshallable.json")

	// Channels cannot be marshalled to JSON
	ch := make(chan int)
	err := AtomicWriteJSON(testFile, ch)
	if err == nil {
		t.Fatal("Expected error for unmarshallable type")
	}

	// Verify file was not created
	if _, statErr := os.Stat(testFile); !os.IsNotExist(statErr) {
		t.Fatal("File should not exist after marshal error")
	}

	// Verify temp file was not left behind
	tmpFile := testFile + ".tmp"
	if _, statErr := os.Stat(tmpFile); !os.IsNotExist(statErr) {
		t.Fatal("Temp file should not exist after marshal error")
	}
}

func TestAtomicWriteFileReadOnlyDir(t *testing.T) {
	tmpDir := t.TempDir()
	roDir := filepath.Join(tmpDir, "readonly")

	// Create read-only directory
	if err := os.Mkdir(roDir, 0555); err != nil {
		t.Fatalf("Failed to create readonly dir: %v", err)
	}
	defer os.Chmod(roDir, 0755) // Restore permissions for cleanup

	testFile := filepath.Join(roDir, "test.txt")
	err := AtomicWriteFile(testFile, []byte("test"), 0644)
	if err == nil {
		t.Fatal("Expected permission error")
	}

	// Verify no files were created
	if _, statErr := os.Stat(testFile); !os.IsNotExist(statErr) {
		t.Fatal("File should not exist after permission error")
	}
}

func TestAtomicWriteFileConcurrent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "concurrent.txt")

	// Write initial content
	if err := AtomicWriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("Initial write error: %v", err)
	}

	// Concurrent writes
	const numWriters = 10
	var wg sync.WaitGroup
	wg.Add(numWriters)

	for i := 0; i < numWriters; i++ {
		go func(n int) {
			defer wg.Done()
			data := []byte(string(rune('A' + n)))
			// Errors are possible due to race, but file should remain valid
			_ = AtomicWriteFile(testFile, data, 0644)
		}(i)
	}

	wg.Wait()

	// Verify file is readable and contains valid content (one of the writes won)
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(content) != 1 {
		t.Errorf("Expected single character, got %q", content)
	}

	// Verify no temp files left behind
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir error: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("Temp file left behind: %s", e.Name())
		}
	}
}

func TestAtomicWritePreservesOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "preserve.txt")

	// Write initial content
	initialContent := []byte("original content")
	if err := AtomicWriteFile(testFile, initialContent, 0644); err != nil {
		t.Fatalf("Initial write error: %v", err)
	}

	// Create a subdirectory with the .tmp name to cause rename to fail
	tmpFile := testFile + ".tmp"
	if err := os.Mkdir(tmpFile, 0755); err != nil {
		t.Fatalf("Failed to create blocking dir: %v", err)
	}

	// Attempt write which should fail at rename
	err := AtomicWriteFile(testFile, []byte("new content"), 0644)
	if err == nil {
		t.Fatal("Expected error when .tmp is a directory")
	}

	// Verify original content is preserved
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(content) != string(initialContent) {
		t.Errorf("Original content not preserved: got %q", content)
	}
}

func TestAtomicWriteJSONStruct(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "struct.json")

	type TestStruct struct {
		Name    string   `json:"name"`
		Count   int      `json:"count"`
		Enabled bool     `json:"enabled"`
		Tags    []string `json:"tags"`
	}

	data := TestStruct{
		Name:    "test",
		Count:   42,
		Enabled: true,
		Tags:    []string{"a", "b"},
	}

	if err := AtomicWriteJSON(testFile, data); err != nil {
		t.Fatalf("AtomicWriteJSON error: %v", err)
	}

	// Read back and verify
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var result TestStruct
	if err := json.Unmarshal(content, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if result.Name != data.Name || result.Count != data.Count ||
		result.Enabled != data.Enabled || len(result.Tags) != len(data.Tags) {
		t.Errorf("Data mismatch: got %+v, want %+v", result, data)
	}
}

func TestAtomicWriteFileLargeData(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.bin")

	// Create 1MB of data
	size := 1024 * 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	if err := AtomicWriteFile(testFile, data, 0644); err != nil {
		t.Fatalf("AtomicWriteFile error: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if len(content) != size {
		t.Errorf("Size mismatch: got %d, want %d", len(content), size)
	}
	for i := 0; i < size; i++ {
		if content[i] != byte(i%256) {
			t.Errorf("Content mismatch at byte %d", i)
			break
		}
	}
}

package util

import (
	"os"
	"path/filepath"
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

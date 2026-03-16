package posting

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead_NoFile(t *testing.T) {
	dir := t.TempDir()
	got := Read(dir)
	if got != "" {
		t.Errorf("Read() with no posting file = %q, want empty", got)
	}
}

func TestRead_EmptyDir(t *testing.T) {
	got := Read("")
	if got != "" {
		t.Errorf("Read(\"\") = %q, want empty", got)
	}
}

func TestRead_WithPosting(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "posting"), []byte("scout\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := Read(dir)
	if got != "scout" {
		t.Errorf("Read() = %q, want %q", got, "scout")
	}
}

func TestWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "dispatcher"); err != nil {
		t.Fatal(err)
	}

	got := Read(dir)
	if got != "dispatcher" {
		t.Errorf("after Write, Read() = %q, want %q", got, "dispatcher")
	}
}

func TestClear_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	if err := Write(dir, "scout"); err != nil {
		t.Fatal(err)
	}
	if err := Clear(dir); err != nil {
		t.Fatal(err)
	}

	got := Read(dir)
	if got != "" {
		t.Errorf("after Clear, Read() = %q, want empty", got)
	}
}

func TestAppendBracket(t *testing.T) {
	tests := []struct {
		base    string
		posting string
		want    string
	}{
		{"gastown/crew/diesel", "scout", "gastown/crew/diesel[scout]"},
		{"gastown/polecats/furiosa", "dispatcher", "gastown/polecats/furiosa[dispatcher]"},
		{"gastown/crew/diesel", "", "gastown/crew/diesel"},
		{"", "scout", "[scout]"},
	}

	for _, tt := range tests {
		got := AppendBracket(tt.base, tt.posting)
		if got != tt.want {
			t.Errorf("AppendBracket(%q, %q) = %q, want %q", tt.base, tt.posting, got, tt.want)
		}
	}
}

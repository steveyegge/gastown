package connection

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBasicFileInfo(t *testing.T) {
	now := time.Now()
	fi := BasicFileInfo{
		FileName:    "test.txt",
		FileSize:    1024,
		FileMode:    0644,
		FileModTime: now,
		FileIsDir:   false,
	}

	if fi.Name() != "test.txt" {
		t.Errorf("Name() = %q, want %q", fi.Name(), "test.txt")
	}
	if fi.Size() != 1024 {
		t.Errorf("Size() = %d, want 1024", fi.Size())
	}
	if fi.Mode() != fs.FileMode(0644) {
		t.Errorf("Mode() = %v, want 0644", fi.Mode())
	}
	if !fi.ModTime().Equal(now) {
		t.Errorf("ModTime() = %v, want %v", fi.ModTime(), now)
	}
	if fi.IsDir() {
		t.Error("IsDir() = true, want false")
	}

	// Directory variant
	dirInfo := BasicFileInfo{
		FileName:  "mydir",
		FileIsDir: true,
		FileMode:  fs.ModeDir | 0755,
	}
	if !dirInfo.IsDir() {
		t.Error("IsDir() = false for directory")
	}
	if dirInfo.Mode()&fs.ModeDir == 0 {
		t.Error("Mode() missing ModeDir bit")
	}
}

func TestBasicFileInfo_ImplementsFileInfo(t *testing.T) {
	var _ FileInfo = BasicFileInfo{}
}

func TestConnectionError(t *testing.T) {
	inner := errors.New("connection refused")
	ce := &ConnectionError{
		Op:      "connect",
		Machine: "remote1",
		Err:     inner,
	}

	expected := "connection connect on remote1: connection refused"
	if ce.Error() != expected {
		t.Errorf("Error() = %q, want %q", ce.Error(), expected)
	}

	if !errors.Is(ce, inner) {
		t.Error("Unwrap() should allow errors.Is to find inner error")
	}
}

func TestNotFoundError(t *testing.T) {
	nfe := &NotFoundError{Path: "/tmp/missing.txt"}
	expected := "not found: /tmp/missing.txt"
	if nfe.Error() != expected {
		t.Errorf("Error() = %q, want %q", nfe.Error(), expected)
	}
}

func TestPermissionError(t *testing.T) {
	pe := &PermissionError{Path: "/etc/shadow", Op: "read"}
	expected := "permission denied: read /etc/shadow"
	if pe.Error() != expected {
		t.Errorf("Error() = %q, want %q", pe.Error(), expected)
	}
}

func TestFromOSFileInfo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fromosfi.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	osFi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	bfi := FromOSFileInfo(osFi)

	if bfi.Name() != osFi.Name() {
		t.Errorf("Name() = %q, want %q", bfi.Name(), osFi.Name())
	}
	if bfi.Size() != osFi.Size() {
		t.Errorf("Size() = %d, want %d", bfi.Size(), osFi.Size())
	}
	if bfi.Mode() != osFi.Mode() {
		t.Errorf("Mode() = %v, want %v", bfi.Mode(), osFi.Mode())
	}
	if !bfi.ModTime().Equal(osFi.ModTime()) {
		t.Errorf("ModTime mismatch")
	}
	if bfi.IsDir() != osFi.IsDir() {
		t.Errorf("IsDir() = %v, want %v", bfi.IsDir(), osFi.IsDir())
	}
}

package connection

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLocalConnection_NameAndIsLocal(t *testing.T) {
	c := NewLocalConnection()
	if c.Name() != "local" {
		t.Errorf("Name() = %q, want %q", c.Name(), "local")
	}
	if !c.IsLocal() {
		t.Error("IsLocal() = false, want true")
	}
}

func TestLocalConnection_ReadFile(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("existing file", func(t *testing.T) {
		path := filepath.Join(dir, "hello.txt")
		if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}
		data, err := c.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(data) != "hello world" {
			t.Errorf("ReadFile() = %q, want %q", data, "hello world")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := c.ReadFile(filepath.Join(dir, "nonexistent"))
		if err == nil {
			t.Fatal("ReadFile() expected error for nonexistent file")
		}
		var nfe *NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("empty file", func(t *testing.T) {
		path := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
		data, err := c.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if len(data) != 0 {
			t.Errorf("ReadFile() = %q, want empty", data)
		}
	})
}

func TestLocalConnection_WriteFile(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("create new file", func(t *testing.T) {
		path := filepath.Join(dir, "new.txt")
		if err := c.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "content" {
			t.Errorf("file content = %q, want %q", data, "content")
		}
	})

	t.Run("overwrite existing", func(t *testing.T) {
		path := filepath.Join(dir, "overwrite.txt")
		if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := c.WriteFile(path, []byte("new"), 0644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "new" {
			t.Errorf("file content = %q, want %q", data, "new")
		}
	})

	t.Run("write to nonexistent parent", func(t *testing.T) {
		path := filepath.Join(dir, "no", "such", "dir", "file.txt")
		err := c.WriteFile(path, []byte("data"), 0644)
		if err == nil {
			t.Fatal("WriteFile() expected error for nonexistent parent")
		}
	})
}

func TestLocalConnection_MkdirAll(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("create nested dirs", func(t *testing.T) {
		path := filepath.Join(dir, "a", "b", "c")
		if err := c.MkdirAll(path, 0755); err != nil {
			t.Fatalf("MkdirAll() error = %v", err)
		}
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat after MkdirAll: %v", err)
		}
		if !fi.IsDir() {
			t.Error("expected directory")
		}
	})

	t.Run("idempotent", func(t *testing.T) {
		path := filepath.Join(dir, "existing")
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := c.MkdirAll(path, 0755); err != nil {
			t.Fatalf("MkdirAll() on existing dir error = %v", err)
		}
	})
}

func TestLocalConnection_Remove(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("remove file", func(t *testing.T) {
		path := filepath.Join(dir, "removeme.txt")
		if err := os.WriteFile(path, []byte("bye"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := c.Remove(path); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("file still exists after Remove")
		}
	})

	t.Run("remove nonexistent is no-op", func(t *testing.T) {
		if err := c.Remove(filepath.Join(dir, "nope")); err != nil {
			t.Fatalf("Remove() nonexistent error = %v", err)
		}
	})

	t.Run("remove empty dir", func(t *testing.T) {
		path := filepath.Join(dir, "emptydir")
		if err := os.Mkdir(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := c.Remove(path); err != nil {
			t.Fatalf("Remove() empty dir error = %v", err)
		}
	})
}

func TestLocalConnection_RemoveAll(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("remove tree", func(t *testing.T) {
		base := filepath.Join(dir, "tree")
		sub := filepath.Join(base, "sub")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "file.txt"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := c.RemoveAll(base); err != nil {
			t.Fatalf("RemoveAll() error = %v", err)
		}
		if _, err := os.Stat(base); !os.IsNotExist(err) {
			t.Error("directory tree still exists after RemoveAll")
		}
	})

	t.Run("remove nonexistent is no-op", func(t *testing.T) {
		if err := c.RemoveAll(filepath.Join(dir, "ghost")); err != nil {
			t.Fatalf("RemoveAll() nonexistent error = %v", err)
		}
	})
}

func TestLocalConnection_Stat(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("stat file", func(t *testing.T) {
		path := filepath.Join(dir, "statme.txt")
		if err := os.WriteFile(path, []byte("12345"), 0644); err != nil {
			t.Fatal(err)
		}
		fi, err := c.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if fi.Name() != "statme.txt" {
			t.Errorf("Name() = %q, want %q", fi.Name(), "statme.txt")
		}
		if fi.Size() != 5 {
			t.Errorf("Size() = %d, want 5", fi.Size())
		}
		if fi.IsDir() {
			t.Error("IsDir() = true, want false")
		}
	})

	t.Run("stat directory", func(t *testing.T) {
		sub := filepath.Join(dir, "subdir")
		if err := os.Mkdir(sub, 0755); err != nil {
			t.Fatal(err)
		}
		fi, err := c.Stat(sub)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if !fi.IsDir() {
			t.Error("IsDir() = false, want true")
		}
	})

	t.Run("stat not found", func(t *testing.T) {
		_, err := c.Stat(filepath.Join(dir, "nope"))
		if err == nil {
			t.Fatal("Stat() expected error for nonexistent")
		}
		var nfe *NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})
}

func TestLocalConnection_Glob(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	// Create test files
	for _, name := range []string{"a.txt", "b.txt", "c.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("match txt files", func(t *testing.T) {
		matches, err := c.Glob(filepath.Join(dir, "*.txt"))
		if err != nil {
			t.Fatalf("Glob() error = %v", err)
		}
		if len(matches) != 2 {
			t.Errorf("Glob() returned %d matches, want 2", len(matches))
		}
	})

	t.Run("match all", func(t *testing.T) {
		matches, err := c.Glob(filepath.Join(dir, "*"))
		if err != nil {
			t.Fatalf("Glob() error = %v", err)
		}
		if len(matches) != 3 {
			t.Errorf("Glob() returned %d matches, want 3", len(matches))
		}
	})

	t.Run("no matches", func(t *testing.T) {
		matches, err := c.Glob(filepath.Join(dir, "*.rs"))
		if err != nil {
			t.Fatalf("Glob() error = %v", err)
		}
		if len(matches) != 0 {
			t.Errorf("Glob() returned %d matches, want 0", len(matches))
		}
	})
}

func TestLocalConnection_Exists(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	path := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("file exists", func(t *testing.T) {
		ok, err := c.Exists(path)
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !ok {
			t.Error("Exists() = false, want true")
		}
	})

	t.Run("file not exists", func(t *testing.T) {
		ok, err := c.Exists(filepath.Join(dir, "nope"))
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if ok {
			t.Error("Exists() = true, want false")
		}
	})

	t.Run("directory exists", func(t *testing.T) {
		ok, err := c.Exists(dir)
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !ok {
			t.Error("Exists() = false for existing dir")
		}
	})
}

func TestLocalConnection_Exec(t *testing.T) {
	c := NewLocalConnection()

	t.Run("echo command", func(t *testing.T) {
		out, err := c.Exec("echo", "hello")
		if err != nil {
			t.Fatalf("Exec() error = %v", err)
		}
		if string(out) != "hello\n" {
			t.Errorf("Exec() output = %q, want %q", out, "hello\n")
		}
	})

	t.Run("failing command", func(t *testing.T) {
		_, err := c.Exec("false")
		if err == nil {
			t.Error("Exec(false) expected error")
		}
	})
}

func TestLocalConnection_ExecDir(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	out, err := c.ExecDir(dir, "pwd")
	if err != nil {
		t.Fatalf("ExecDir() error = %v", err)
	}
	// Resolve symlinks for macOS /private/tmp
	resolvedDir, _ := filepath.EvalSymlinks(dir)
	got := string(out)
	if got != resolvedDir+"\n" && got != dir+"\n" {
		t.Errorf("ExecDir() output = %q, want dir %q", got, resolvedDir)
	}
}

func TestLocalConnection_ExecEnv(t *testing.T) {
	c := NewLocalConnection()

	env := map[string]string{"TEST_CONN_VAR": "42"}
	out, err := c.ExecEnv(env, "sh", "-c", "echo $TEST_CONN_VAR")
	if err != nil {
		t.Fatalf("ExecEnv() error = %v", err)
	}
	if string(out) != "42\n" {
		t.Errorf("ExecEnv() output = %q, want %q", out, "42\n")
	}
}

func TestLocalConnection_ReadFile_PermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root, permission tests unreliable")
	}

	c := NewLocalConnection()
	dir := t.TempDir()

	path := filepath.Join(dir, "noperm.txt")
	if err := os.WriteFile(path, []byte("secret"), 0000); err != nil {
		t.Fatal(err)
	}

	_, err := c.ReadFile(path)
	if err == nil {
		t.Fatal("ReadFile() expected permission error")
	}
	var pe *PermissionError
	if !errors.As(err, &pe) {
		t.Errorf("expected PermissionError, got %T: %v", err, err)
	}
}

func TestLocalConnection_Stat_PermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("running as root, permission tests unreliable")
	}

	c := NewLocalConnection()
	dir := t.TempDir()

	// Create a directory with no read permission, then try to stat a child
	noread := filepath.Join(dir, "noread")
	if err := os.Mkdir(noread, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(noread, 0755) })

	_, err := c.Stat(filepath.Join(noread, "child"))
	if err == nil {
		t.Fatal("Stat() expected error for permission-denied parent")
	}
	// The error type depends on OS behavior; just ensure we get an error
}

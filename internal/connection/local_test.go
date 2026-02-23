package connection

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalConnection_Name(t *testing.T) {
	c := NewLocalConnection()
	if got := c.Name(); got != "local" {
		t.Errorf("Name() = %q, want %q", got, "local")
	}
}

func TestLocalConnection_IsLocal(t *testing.T) {
	c := NewLocalConnection()
	if !c.IsLocal() {
		t.Error("IsLocal() = false, want true")
	}
}

func TestLocalConnection_ReadFile(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("success", func(t *testing.T) {
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
			t.Fatal(err)
		}
		data, err := c.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error: %v", err)
		}
		if string(data) != "hello" {
			t.Errorf("ReadFile() = %q, want %q", data, "hello")
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
}

func TestLocalConnection_WriteFile(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("success", func(t *testing.T) {
		path := filepath.Join(dir, "write.txt")
		if err := c.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatalf("WriteFile() error: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "data" {
			t.Errorf("file content = %q, want %q", data, "data")
		}
	})
}

func TestLocalConnection_MkdirAll(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	path := filepath.Join(dir, "a", "b", "c")
	if err := c.MkdirAll(path, 0755); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !fi.IsDir() {
		t.Error("expected directory")
	}
}

func TestLocalConnection_Remove(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("remove existing file", func(t *testing.T) {
		path := filepath.Join(dir, "removeme.txt")
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := c.Remove(path); err != nil {
			t.Fatalf("Remove() error: %v", err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("file still exists after Remove")
		}
	})

	t.Run("remove nonexistent is no-op", func(t *testing.T) {
		if err := c.Remove(filepath.Join(dir, "gone")); err != nil {
			t.Errorf("Remove() nonexistent should not error, got: %v", err)
		}
	})
}

func TestLocalConnection_RemoveAll(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "deep"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "deep", "f.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveAll(sub); err != nil {
		t.Fatalf("RemoveAll() error: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Error("directory still exists after RemoveAll")
	}
}

func TestLocalConnection_Stat(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	t.Run("existing file", func(t *testing.T) {
		path := filepath.Join(dir, "statme.txt")
		if err := os.WriteFile(path, []byte("abc"), 0644); err != nil {
			t.Fatal(err)
		}
		fi, err := c.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error: %v", err)
		}
		if fi.Name() != "statme.txt" {
			t.Errorf("Name() = %q, want %q", fi.Name(), "statme.txt")
		}
		if fi.Size() != 3 {
			t.Errorf("Size() = %d, want 3", fi.Size())
		}
		if fi.IsDir() {
			t.Error("IsDir() = true, want false")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := c.Stat(filepath.Join(dir, "nope"))
		if err == nil {
			t.Fatal("Stat() expected error for nonexistent")
		}
		var nfe *NotFoundError
		if !errors.As(err, &nfe) {
			t.Errorf("expected NotFoundError, got %T: %v", err, err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		fi, err := c.Stat(dir)
		if err != nil {
			t.Fatalf("Stat() error: %v", err)
		}
		if !fi.IsDir() {
			t.Error("IsDir() = false, want true for directory")
		}
	})
}

func TestLocalConnection_Glob(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	for _, name := range []string{"a.txt", "b.txt", "c.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	matches, err := c.Glob(filepath.Join(dir, "*.txt"))
	if err != nil {
		t.Fatalf("Glob() error: %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("Glob(*.txt) matched %d files, want 2", len(matches))
	}
}

func TestLocalConnection_Exists(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()

	path := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("existing", func(t *testing.T) {
		exists, err := c.Exists(path)
		if err != nil {
			t.Fatalf("Exists() error: %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true")
		}
	})

	t.Run("nonexistent", func(t *testing.T) {
		exists, err := c.Exists(filepath.Join(dir, "nope"))
		if err != nil {
			t.Fatalf("Exists() error: %v", err)
		}
		if exists {
			t.Error("Exists() = true, want false")
		}
	})
}

func TestLocalConnection_Exec(t *testing.T) {
	c := NewLocalConnection()
	out, err := c.Exec("echo", "hello")
	if err != nil {
		t.Fatalf("Exec() error: %v", err)
	}
	if got := string(out); got != "hello\n" {
		t.Errorf("Exec() output = %q, want %q", got, "hello\n")
	}
}

func TestLocalConnection_ExecDir(t *testing.T) {
	c := NewLocalConnection()
	dir := t.TempDir()
	out, err := c.ExecDir(dir, "pwd")
	if err != nil {
		t.Fatalf("ExecDir() error: %v", err)
	}
	// pwd output should contain the temp dir path
	if got := string(out); got == "" {
		t.Error("ExecDir() returned empty output")
	}
}

func TestLocalConnection_ExecEnv(t *testing.T) {
	c := NewLocalConnection()
	env := map[string]string{"GT_TEST_VAR": "test_value_42"}
	out, err := c.ExecEnv(env, "sh", "-c", "echo $GT_TEST_VAR")
	if err != nil {
		t.Fatalf("ExecEnv() error: %v", err)
	}
	if got := string(out); got != "test_value_42\n" {
		t.Errorf("ExecEnv() output = %q, want %q", got, "test_value_42\n")
	}
}

func TestLocalConnection_ImplementsInterface(t *testing.T) {
	var _ Connection = (*LocalConnection)(nil)
}

// Error type tests

func TestNotFoundError(t *testing.T) {
	err := &NotFoundError{Path: "/foo/bar"}
	if got := err.Error(); got != "not found: /foo/bar" {
		t.Errorf("Error() = %q, want %q", got, "not found: /foo/bar")
	}
}

func TestPermissionError(t *testing.T) {
	err := &PermissionError{Path: "/secret", Op: "read"}
	if got := err.Error(); got != "permission denied: read /secret" {
		t.Errorf("Error() = %q, want %q", got, "permission denied: read /secret")
	}
}

func TestConnectionError(t *testing.T) {
	inner := errors.New("timeout")
	err := &ConnectionError{Op: "connect", Machine: "remote1", Err: inner}

	if got := err.Error(); got != "connection connect on remote1: timeout" {
		t.Errorf("Error() = %q", got)
	}

	if unwrapped := err.Unwrap(); unwrapped != inner {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, inner)
	}
}

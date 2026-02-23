package connection

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewMachineRegistry_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatalf("NewMachineRegistry() error: %v", err)
	}

	// Should always have "local" machine
	m, err := r.Get("local")
	if err != nil {
		t.Fatalf("Get(local) error: %v", err)
	}
	if m.Name != "local" || m.Type != "local" {
		t.Errorf("local machine = %+v", m)
	}
}

func TestNewMachineRegistry_ExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	config := `{"version":1,"machines":{"local":{"name":"local","type":"local"},"remote1":{"name":"remote1","type":"ssh","host":"user@host"}}}`
	if err := os.WriteFile(path, []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatalf("NewMachineRegistry() error: %v", err)
	}

	m, err := r.Get("remote1")
	if err != nil {
		t.Fatalf("Get(remote1) error: %v", err)
	}
	if m.Host != "user@host" {
		t.Errorf("Host = %q, want %q", m.Host, "user@host")
	}
}

func TestNewMachineRegistry_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewMachineRegistry(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMachineRegistry_Add(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("valid ssh machine", func(t *testing.T) {
		m := &Machine{Name: "remote1", Type: "ssh", Host: "user@host"}
		if err := r.Add(m); err != nil {
			t.Fatalf("Add() error: %v", err)
		}
		got, err := r.Get("remote1")
		if err != nil {
			t.Fatalf("Get() error: %v", err)
		}
		if got.Host != "user@host" {
			t.Errorf("Host = %q, want %q", got.Host, "user@host")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		m := &Machine{Type: "local"}
		if err := r.Add(m); err == nil {
			t.Error("Add() with empty name should error")
		}
	})

	t.Run("empty type", func(t *testing.T) {
		m := &Machine{Name: "x"}
		if err := r.Add(m); err == nil {
			t.Error("Add() with empty type should error")
		}
	})

	t.Run("ssh without host", func(t *testing.T) {
		m := &Machine{Name: "bad", Type: "ssh"}
		if err := r.Add(m); err == nil {
			t.Error("Add() ssh without host should error")
		}
	})
}

func TestMachineRegistry_Remove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	// Add then remove
	if err := r.Add(&Machine{Name: "temp", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Remove("temp"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}
	if _, err := r.Get("temp"); err == nil {
		t.Error("Get() should fail after Remove()")
	}

	t.Run("cannot remove local", func(t *testing.T) {
		if err := r.Remove("local"); err == nil {
			t.Error("Remove(local) should error")
		}
	})

	t.Run("remove nonexistent", func(t *testing.T) {
		if err := r.Remove("nope"); err == nil {
			t.Error("Remove(nope) should error")
		}
	})
}

func TestMachineRegistry_List(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := r.Add(&Machine{Name: "a", Type: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(&Machine{Name: "b", Type: "local"}); err != nil {
		t.Fatal(err)
	}

	list := r.List()
	// Should have local + a + b = 3
	if len(list) != 3 {
		t.Errorf("List() returned %d machines, want 3", len(list))
	}
}

func TestMachineRegistry_Connection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("local connection", func(t *testing.T) {
		conn, err := r.Connection("local")
		if err != nil {
			t.Fatalf("Connection(local) error: %v", err)
		}
		if !conn.IsLocal() {
			t.Error("expected local connection")
		}
	})

	t.Run("ssh not implemented", func(t *testing.T) {
		if err := r.Add(&Machine{Name: "ssh1", Type: "ssh", Host: "user@host"}); err != nil {
			t.Fatal(err)
		}
		_, err := r.Connection("ssh1")
		if err == nil {
			t.Error("Connection(ssh) should error (not implemented)")
		}
	})

	t.Run("unknown machine", func(t *testing.T) {
		_, err := r.Connection("nope")
		if err == nil {
			t.Error("Connection(nope) should error")
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		// Directly inject a machine with unknown type
		r.mu.Lock()
		r.machines["weird"] = &Machine{Name: "weird", Type: "quantum"}
		r.mu.Unlock()

		_, err := r.Connection("weird")
		if err == nil {
			t.Error("Connection(quantum) should error")
		}
	})
}

func TestMachineRegistry_LocalConnection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	conn := r.LocalConnection()
	if conn == nil {
		t.Fatal("LocalConnection() returned nil")
	}
	if !conn.IsLocal() {
		t.Error("expected local connection")
	}
}

func TestMachineRegistry_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	// Create and add a machine
	r1, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r1.Add(&Machine{Name: "persist", Type: "ssh", Host: "u@h", KeyPath: "/key"}); err != nil {
		t.Fatal(err)
	}

	// Load from same file — should see the machine
	r2, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := r2.Get("persist")
	if err != nil {
		t.Fatalf("persisted machine not found: %v", err)
	}
	if m.Host != "u@h" || m.KeyPath != "/key" {
		t.Errorf("persisted machine = %+v", m)
	}
}

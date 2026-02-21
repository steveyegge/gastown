package connection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewMachineRegistry_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatalf("NewMachineRegistry() error = %v", err)
	}

	// Should always have "local"
	m, err := r.Get("local")
	if err != nil {
		t.Fatalf("Get(local) error = %v", err)
	}
	if m.Type != "local" {
		t.Errorf("local machine type = %q, want %q", m.Type, "local")
	}
}

func TestNewMachineRegistry_ExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	data := registryData{
		Version: 1,
		Machines: map[string]*Machine{
			"remote1": {Name: "remote1", Type: "ssh", Host: "user@host1"},
		},
	}
	raw, _ := json.Marshal(data)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatalf("NewMachineRegistry() error = %v", err)
	}

	// Should have both local and remote1
	if _, err := r.Get("local"); err != nil {
		t.Errorf("Get(local) error = %v", err)
	}
	m, err := r.Get("remote1")
	if err != nil {
		t.Fatalf("Get(remote1) error = %v", err)
	}
	if m.Host != "user@host1" {
		t.Errorf("remote1 host = %q, want %q", m.Host, "user@host1")
	}
}

func TestNewMachineRegistry_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	if err := os.WriteFile(path, []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := NewMachineRegistry(path)
	if err == nil {
		t.Fatal("NewMachineRegistry() expected error for invalid JSON")
	}
}

func TestMachineRegistry_Add(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("add ssh machine", func(t *testing.T) {
		m := &Machine{Name: "vm1", Type: "ssh", Host: "user@vm1.example.com"}
		if err := r.Add(m); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		got, err := r.Get("vm1")
		if err != nil {
			t.Fatalf("Get(vm1) error = %v", err)
		}
		if got.Host != "user@vm1.example.com" {
			t.Errorf("Host = %q, want %q", got.Host, "user@vm1.example.com")
		}

		// Verify persisted to disk
		r2, err := NewMachineRegistry(path)
		if err != nil {
			t.Fatal(err)
		}
		got2, err := r2.Get("vm1")
		if err != nil {
			t.Fatalf("reloaded Get(vm1) error = %v", err)
		}
		if got2.Host != "user@vm1.example.com" {
			t.Errorf("reloaded Host = %q, want %q", got2.Host, "user@vm1.example.com")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		err := r.Add(&Machine{Type: "local"})
		if err == nil {
			t.Error("Add() expected error for empty name")
		}
	})

	t.Run("empty type rejected", func(t *testing.T) {
		err := r.Add(&Machine{Name: "test"})
		if err == nil {
			t.Error("Add() expected error for empty type")
		}
	})

	t.Run("ssh without host rejected", func(t *testing.T) {
		err := r.Add(&Machine{Name: "bad-ssh", Type: "ssh"})
		if err == nil {
			t.Error("Add() expected error for ssh without host")
		}
	})

	t.Run("update existing", func(t *testing.T) {
		m := &Machine{Name: "vm1", Type: "ssh", Host: "newuser@vm1.example.com"}
		if err := r.Add(m); err != nil {
			t.Fatalf("Add() update error = %v", err)
		}
		got, _ := r.Get("vm1")
		if got.Host != "newuser@vm1.example.com" {
			t.Errorf("updated Host = %q", got.Host)
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

	// Add a machine first
	r.Add(&Machine{Name: "removeme", Type: "local"})

	t.Run("remove existing", func(t *testing.T) {
		if err := r.Remove("removeme"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}
		_, err := r.Get("removeme")
		if err == nil {
			t.Error("Get() after Remove should error")
		}
	})

	t.Run("remove local rejected", func(t *testing.T) {
		err := r.Remove("local")
		if err == nil {
			t.Error("Remove(local) should be rejected")
		}
	})

	t.Run("remove nonexistent", func(t *testing.T) {
		err := r.Remove("ghost")
		if err == nil {
			t.Error("Remove(ghost) should error")
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

	// Should have at least "local"
	machines := r.List()
	if len(machines) < 1 {
		t.Fatal("List() returned empty, expected at least local")
	}

	found := false
	for _, m := range machines {
		if m.Name == "local" {
			found = true
			break
		}
	}
	if !found {
		t.Error("List() missing local machine")
	}

	// Add another and verify count
	r.Add(&Machine{Name: "extra", Type: "local"})
	machines = r.List()
	if len(machines) != 2 {
		t.Errorf("List() returned %d, want 2", len(machines))
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
			t.Fatalf("Connection(local) error = %v", err)
		}
		if !conn.IsLocal() {
			t.Error("Connection(local).IsLocal() = false")
		}
	})

	t.Run("ssh not implemented", func(t *testing.T) {
		r.Add(&Machine{Name: "sshbox", Type: "ssh", Host: "user@host"})
		_, err := r.Connection("sshbox")
		if err == nil {
			t.Error("Connection(sshbox) should error (ssh not implemented)")
		}
	})

	t.Run("unknown machine", func(t *testing.T) {
		_, err := r.Connection("nope")
		if err == nil {
			t.Error("Connection(nope) should error")
		}
	})

	t.Run("unknown type", func(t *testing.T) {
		r.Add(&Machine{Name: "weirdtype", Type: "carrier-pigeon"})
		_, err := r.Connection("weirdtype")
		if err == nil {
			t.Error("Connection(weirdtype) should error for unknown type")
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

	lc := r.LocalConnection()
	if lc == nil {
		t.Fatal("LocalConnection() returned nil")
	}
	if lc.Name() != "local" {
		t.Errorf("LocalConnection().Name() = %q", lc.Name())
	}
}

func TestMachineRegistry_NamePopulation(t *testing.T) {
	// Verify that machine names are populated from map keys on load
	dir := t.TempDir()
	path := filepath.Join(dir, "machines.json")

	// Write config where machine name field is empty (names come from keys)
	data := registryData{
		Version: 1,
		Machines: map[string]*Machine{
			"mybox": {Type: "ssh", Host: "user@mybox"},
		},
	}
	raw, _ := json.Marshal(data)
	os.WriteFile(path, raw, 0644)

	r, err := NewMachineRegistry(path)
	if err != nil {
		t.Fatal(err)
	}

	m, err := r.Get("mybox")
	if err != nil {
		t.Fatal(err)
	}
	if m.Name != "mybox" {
		t.Errorf("Name = %q, want %q (should be populated from map key)", m.Name, "mybox")
	}
}

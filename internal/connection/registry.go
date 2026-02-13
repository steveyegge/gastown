package connection

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Machine represents a managed machine in the federation.
type Machine struct {
	Name     string `json:"name"`
	Type     string `json:"type"`      // "k8s"
	Host     string `json:"host"`      // for future use
	KeyPath  string `json:"key_path"`  // for future use
	TownPath string `json:"town_path"` // Path to town root on remote
}

// registryData is the JSON file structure.
type registryData struct {
	Version  int                 `json:"version"`
	Machines map[string]*Machine `json:"machines"`
}

// MachineRegistry manages machine configurations and provides Connection instances.
type MachineRegistry struct {
	path     string
	machines map[string]*Machine
	mu       sync.RWMutex
}

// NewMachineRegistry creates a registry from the given config file path.
// If the file doesn't exist, an empty registry is created.
func NewMachineRegistry(configPath string) (*MachineRegistry, error) {
	r := &MachineRegistry{
		path:     configPath,
		machines: make(map[string]*Machine),
	}

	// Load existing config if present
	if err := r.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading registry: %w", err)
	}

	return r, nil
}

// load reads the registry from disk.
func (r *MachineRegistry) load() error {
	data, err := os.ReadFile(r.path)
	if err != nil {
		return err
	}

	var rd registryData
	if err := json.Unmarshal(data, &rd); err != nil {
		return fmt.Errorf("parsing registry: %w", err)
	}

	r.machines = rd.Machines
	if r.machines == nil {
		r.machines = make(map[string]*Machine)
	}

	// Populate machine names from keys
	for name, m := range r.machines {
		m.Name = name
	}

	return nil
}

// save writes the registry to disk.
func (r *MachineRegistry) save() error {
	rd := registryData{
		Version:  1,
		Machines: r.machines,
	}

	data, err := json.MarshalIndent(rd, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling registry: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(r.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(r.path, data, fs.FileMode(0644)); err != nil {
		return fmt.Errorf("writing registry: %w", err)
	}

	return nil
}

// Get returns a machine by name.
func (r *MachineRegistry) Get(name string) (*Machine, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.machines[name]
	if !ok {
		return nil, fmt.Errorf("machine not found: %s", name)
	}
	return m, nil
}

// Add adds or updates a machine in the registry.
func (r *MachineRegistry) Add(m *Machine) error {
	if m.Name == "" {
		return fmt.Errorf("machine name is required")
	}
	if m.Type == "" {
		return fmt.Errorf("machine type is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.machines[m.Name] = m
	return r.save()
}

// Remove removes a machine from the registry.
func (r *MachineRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.machines[name]; !ok {
		return fmt.Errorf("machine not found: %s", name)
	}

	delete(r.machines, name)
	return r.save()
}

// List returns all machines in the registry.
func (r *MachineRegistry) List() []*Machine {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Machine, 0, len(r.machines))
	for _, m := range r.machines {
		result = append(result, m)
	}
	return result
}

// Connection returns a Connection for the named machine.
func (r *MachineRegistry) Connection(name string) (Connection, error) {
	m, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	switch m.Type {
	case "k8s":
		return nil, fmt.Errorf("k8s connection requires pod configuration; use NewK8sConnection directly")
	default:
		return nil, fmt.Errorf("unknown machine type: %s", m.Type)
	}
}

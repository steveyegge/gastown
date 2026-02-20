package wasteland

import (
	"fmt"
	"sync"
)

// CallLog is a shared ordered log for recording cross-component call sequences.
type CallLog struct {
	mu    sync.Mutex
	Calls []string
}

func NewCallLog() *CallLog {
	return &CallLog{}
}

func (l *CallLog) Record(call string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Calls = append(l.Calls, call)
}

// FakeDoltHubAPI is a test double for DoltHubAPI.
type FakeDoltHubAPI struct {
	mu     sync.Mutex
	Forked map[string]bool // "fromOrg/fromDB -> toOrg" => true
	Calls  []string
	Log    *CallLog // shared ordered log (optional)

	ForkErr error
}

func NewFakeDoltHubAPI() *FakeDoltHubAPI {
	return &FakeDoltHubAPI{Forked: make(map[string]bool)}
}

func (f *FakeDoltHubAPI) ForkRepo(fromOrg, fromDB, toOrg, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fmt.Sprintf("ForkRepo(%s, %s, %s)", fromOrg, fromDB, toOrg)
	f.Calls = append(f.Calls, call)
	if f.Log != nil {
		f.Log.Record(call)
	}
	if f.ForkErr != nil {
		return f.ForkErr
	}
	f.Forked[fmt.Sprintf("%s/%s->%s", fromOrg, fromDB, toOrg)] = true
	return nil
}

// FakeDoltCLI is a test double for DoltCLI.
type FakeDoltCLI struct {
	mu         sync.Mutex
	Cloned     map[string]bool // "org/db -> targetDir"
	Registered map[string]bool // "handle"
	Pushed     map[string]bool // "localDir"
	Remotes    map[string]bool // "localDir -> upstreamOrg/upstreamDB"
	Calls      []string
	Log        *CallLog // shared ordered log (optional)

	CloneErr    error
	RegisterErr error
	PushErr     error
	RemoteErr   error
}

func NewFakeDoltCLI() *FakeDoltCLI {
	return &FakeDoltCLI{
		Cloned:     make(map[string]bool),
		Registered: make(map[string]bool),
		Pushed:     make(map[string]bool),
		Remotes:    make(map[string]bool),
	}
}

func (f *FakeDoltCLI) Clone(org, db, targetDir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fmt.Sprintf("Clone(%s, %s, %s)", org, db, targetDir)
	f.Calls = append(f.Calls, call)
	if f.Log != nil {
		f.Log.Record(call)
	}
	if f.CloneErr != nil {
		return f.CloneErr
	}
	f.Cloned[fmt.Sprintf("%s/%s->%s", org, db, targetDir)] = true
	return nil
}

func (f *FakeDoltCLI) RegisterRig(localDir, handle, dolthubOrg, displayName, ownerEmail, gtVersion string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fmt.Sprintf("RegisterRig(%s, %s)", localDir, handle)
	f.Calls = append(f.Calls, call)
	if f.Log != nil {
		f.Log.Record(call)
	}
	if f.RegisterErr != nil {
		return f.RegisterErr
	}
	f.Registered[handle] = true
	return nil
}

func (f *FakeDoltCLI) Push(localDir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fmt.Sprintf("Push(%s)", localDir)
	f.Calls = append(f.Calls, call)
	if f.Log != nil {
		f.Log.Record(call)
	}
	if f.PushErr != nil {
		return f.PushErr
	}
	f.Pushed[localDir] = true
	return nil
}

func (f *FakeDoltCLI) AddUpstreamRemote(localDir, upstreamOrg, upstreamDB string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := fmt.Sprintf("AddUpstreamRemote(%s, %s, %s)", localDir, upstreamOrg, upstreamDB)
	f.Calls = append(f.Calls, call)
	if f.Log != nil {
		f.Log.Record(call)
	}
	if f.RemoteErr != nil {
		return f.RemoteErr
	}
	f.Remotes[fmt.Sprintf("%s->%s/%s", localDir, upstreamOrg, upstreamDB)] = true
	return nil
}

// FakeConfigStore is a test double for ConfigStore.
type FakeConfigStore struct {
	mu      sync.Mutex
	Configs map[string]*Config // townRoot -> Config

	LoadErr error
	SaveErr error
}

func NewFakeConfigStore() *FakeConfigStore {
	return &FakeConfigStore{Configs: make(map[string]*Config)}
}

func (f *FakeConfigStore) Load(townRoot string) (*Config, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.LoadErr != nil {
		return nil, f.LoadErr
	}
	cfg, ok := f.Configs[townRoot]
	if !ok {
		return nil, ErrNotJoined
	}
	return cfg, nil
}

func (f *FakeConfigStore) Save(townRoot string, cfg *Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.SaveErr != nil {
		return f.SaveErr
	}
	f.Configs[townRoot] = cfg
	return nil
}

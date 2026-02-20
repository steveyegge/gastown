package wasteland

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestJoin_Success(t *testing.T) {
	t.Parallel()
	log := NewCallLog()
	api := NewFakeDoltHubAPI()
	api.Log = log
	cli := NewFakeDoltCLI()
	cli.Log = log
	cfgStore := NewFakeConfigStore()

	svc := &Service{API: api, CLI: cli, Config: cfgStore}

	cfg, err := svc.Join("steveyegge/wl-commons", "alice-dev", "token123", "alice-rig", "Alice", "alice@example.com", "dev", "/tmp/town")
	if err != nil {
		t.Fatalf("Join() error: %v", err)
	}

	// Verify returned config
	if cfg.Upstream != "steveyegge/wl-commons" {
		t.Errorf("Upstream = %q, want %q", cfg.Upstream, "steveyegge/wl-commons")
	}
	if cfg.RigHandle != "alice-rig" {
		t.Errorf("RigHandle = %q, want %q", cfg.RigHandle, "alice-rig")
	}

	// Verify fork happened
	if !api.Forked["steveyegge/wl-commons->alice-dev"] {
		t.Error("expected fork to be created")
	}

	// Verify clone happened
	if len(cli.Cloned) != 1 {
		t.Errorf("expected 1 clone, got %d", len(cli.Cloned))
	}

	// Verify rig registered
	if !cli.Registered["alice-rig"] {
		t.Error("expected rig to be registered")
	}

	// Verify push happened
	if len(cli.Pushed) != 1 {
		t.Errorf("expected 1 push, got %d", len(cli.Pushed))
	}

	// Verify upstream remote added
	if len(cli.Remotes) != 1 {
		t.Errorf("expected 1 remote, got %d", len(cli.Remotes))
	}

	// Verify config saved
	saved, err := cfgStore.Load("/tmp/town")
	if err != nil {
		t.Fatalf("config not saved: %v", err)
	}
	if saved.Upstream != cfg.Upstream {
		t.Errorf("saved config doesn't match returned config")
	}

	// Verify call ordering from unified log: fork, clone, remote, register, push
	expectedOrder := []string{"ForkRepo", "Clone", "AddUpstreamRemote", "RegisterRig", "Push"}
	if len(log.Calls) < len(expectedOrder) {
		t.Fatalf("expected at least %d calls in unified log, got %d: %v", len(expectedOrder), len(log.Calls), log.Calls)
	}
	for i, want := range expectedOrder {
		if i >= len(log.Calls) {
			break
		}
		got := log.Calls[i]
		if !strings.HasPrefix(got, want) {
			t.Errorf("unified log[%d] = %q, want prefix %q", i, got, want)
		}
	}
}

func TestJoin_ForkFails(t *testing.T) {
	t.Parallel()
	api := NewFakeDoltHubAPI()
	api.ForkErr = fmt.Errorf("DoltHub API error (HTTP 403): forbidden")
	cli := NewFakeDoltCLI()
	cfgStore := NewFakeConfigStore()

	svc := &Service{API: api, CLI: cli, Config: cfgStore}

	_, err := svc.Join("steveyegge/wl-commons", "alice-dev", "bad-token", "alice-rig", "Alice", "alice@example.com", "dev", "/tmp/town")
	if err == nil {
		t.Fatal("Join() expected error when fork fails")
	}

	// Verify clone was NOT called
	if len(cli.Cloned) != 0 {
		t.Error("clone should not be called when fork fails")
	}
}

func TestJoin_CloneFails(t *testing.T) {
	t.Parallel()
	api := NewFakeDoltHubAPI()
	cli := NewFakeDoltCLI()
	cli.CloneErr = fmt.Errorf("dolt clone failed: network timeout")
	cfgStore := NewFakeConfigStore()

	svc := &Service{API: api, CLI: cli, Config: cfgStore}

	_, err := svc.Join("steveyegge/wl-commons", "alice-dev", "token", "alice-rig", "Alice", "alice@example.com", "dev", "/tmp/town")
	if err == nil {
		t.Fatal("Join() expected error when clone fails")
	}

	// Fork should have succeeded
	if !api.Forked["steveyegge/wl-commons->alice-dev"] {
		t.Error("fork should have been created before clone failed")
	}

	// Push should NOT have been called
	if len(cli.Pushed) != 0 {
		t.Error("push should not be called when clone fails")
	}
}

func TestJoin_AlreadyJoined(t *testing.T) {
	t.Parallel()
	api := NewFakeDoltHubAPI()
	cli := NewFakeDoltCLI()
	cfgStore := NewFakeConfigStore()

	existing := &Config{
		Upstream:  "steveyegge/wl-commons",
		ForkOrg:   "alice-dev",
		ForkDB:    "wl-commons",
		RigHandle: "alice-rig",
	}
	cfgStore.Configs["/tmp/town"] = existing

	svc := &Service{API: api, CLI: cli, Config: cfgStore}

	cfg, err := svc.Join("steveyegge/wl-commons", "alice-dev", "token", "alice-rig", "Alice", "alice@example.com", "dev", "/tmp/town")
	if err != nil {
		t.Fatalf("Join() should succeed (no-op) when already joined: %v", err)
	}

	// Should return existing config
	if cfg.RigHandle != "alice-rig" {
		t.Errorf("returned config RigHandle = %q, want %q", cfg.RigHandle, "alice-rig")
	}

	// No API calls should have been made
	if len(api.Calls) != 0 {
		t.Errorf("expected 0 API calls for already-joined, got %d", len(api.Calls))
	}
	if len(cli.Calls) != 0 {
		t.Errorf("expected 0 CLI calls for already-joined, got %d", len(cli.Calls))
	}
}

func TestJoin_DifferentUpstreamAlreadyJoined(t *testing.T) {
	t.Parallel()
	api := NewFakeDoltHubAPI()
	cli := NewFakeDoltCLI()
	cfgStore := NewFakeConfigStore()

	existing := &Config{
		Upstream:  "org1/commons",
		ForkOrg:   "alice-dev",
		ForkDB:    "commons",
		RigHandle: "alice-rig",
	}
	cfgStore.Configs["/tmp/town"] = existing

	svc := &Service{API: api, CLI: cli, Config: cfgStore}

	_, err := svc.Join("org2/commons", "alice-dev", "token", "alice-rig", "Alice", "alice@example.com", "dev", "/tmp/town")
	if err == nil {
		t.Fatal("Join() should error when already joined to a different upstream")
	}
	if !strings.Contains(err.Error(), "already joined to org1/commons") {
		t.Errorf("error = %q, want to contain %q", err.Error(), "already joined to org1/commons")
	}

	// No API calls should have been made
	if len(api.Calls) != 0 {
		t.Errorf("expected 0 API calls, got %d", len(api.Calls))
	}
}

func TestJoin_ConfigLoadError(t *testing.T) {
	t.Parallel()
	api := NewFakeDoltHubAPI()
	cli := NewFakeDoltCLI()
	cfgStore := NewFakeConfigStore()
	cfgStore.LoadErr = fmt.Errorf("reading wasteland config: permission denied")

	svc := &Service{API: api, CLI: cli, Config: cfgStore}

	_, err := svc.Join("steveyegge/wl-commons", "alice-dev", "token", "alice-rig", "Alice", "alice@example.com", "dev", "/tmp/town")
	if err == nil {
		t.Fatal("Join() should error when config load fails with non-not-found error")
	}
	if errors.Is(err, ErrNotJoined) {
		t.Error("error should NOT be ErrNotJoined for disk/permission errors")
	}

	// No API calls should have been made — the error should surface immediately
	if len(api.Calls) != 0 {
		t.Errorf("expected 0 API calls, got %d", len(api.Calls))
	}
}

func TestJoin_InvalidUpstream(t *testing.T) {
	t.Parallel()
	svc := &Service{
		API:    NewFakeDoltHubAPI(),
		CLI:    NewFakeDoltCLI(),
		Config: NewFakeConfigStore(),
	}

	_, err := svc.Join("invalid", "org", "token", "handle", "name", "email", "v1", "/tmp/town")
	if err == nil {
		t.Fatal("Join() expected error for invalid upstream")
	}
}

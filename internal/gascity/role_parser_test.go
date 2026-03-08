package gascity

import (
	"strings"
	"testing"
)

func TestParseRoleSpec_UsesProviderDefaults(t *testing.T) {
	spec, err := ParseRoleSpec([]byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "codex"

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/{rig}/crew/{name}"
`))
	if err != nil {
		t.Fatalf("ParseRoleSpec() error = %v", err)
	}

	if spec.Env["GT_ROLE"] != "reviewer" {
		t.Fatalf("GT_ROLE = %q, want reviewer", spec.Env["GT_ROLE"])
	}
	if spec.Env["GT_SCOPE"] != "rig" {
		t.Fatalf("GT_SCOPE = %q, want rig", spec.Env["GT_SCOPE"])
	}
	if got := spec.Session.StartCommand; got != "exec codex --dangerously-bypass-approvals-and-sandbox" {
		t.Fatalf("StartCommand = %q", got)
	}
	if spec.Capabilities.Hooks {
		t.Fatalf("Hooks = true, want false")
	}
	if !spec.Capabilities.Resume {
		t.Fatalf("Resume = false, want true")
	}
	if spec.Capabilities.ForkSession {
		t.Fatalf("ForkSession = true, want false")
	}
	if !spec.Capabilities.Exec {
		t.Fatalf("Exec = false, want true")
	}
	if spec.Capabilities.ReadyStrategy != "delay" {
		t.Fatalf("ReadyStrategy = %q, want delay", spec.Capabilities.ReadyStrategy)
	}
}

func TestParseRoleSpec_RejectsUnknownFields(t *testing.T) {
	_, err := ParseRoleSpec([]byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "codex"
unknown_top_level = true

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/{rig}/crew/{name}"
`))
	if err == nil || !strings.Contains(err.Error(), "unknown fields") {
		t.Fatalf("ParseRoleSpec() error = %v, want unknown field validation", err)
	}
}

func TestParseRoleSpec_RejectsUnknownProvider(t *testing.T) {
	_, err := ParseRoleSpec([]byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "does-not-exist"

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/{rig}/crew/{name}"
`))
	if err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("ParseRoleSpec() error = %v, want unknown provider", err)
	}
}

func TestParseRoleSpec_RejectsRigPlaceholderForTownScope(t *testing.T) {
	_, err := ParseRoleSpec([]byte(`
version = 1
role = "operator"
scope = "town"
provider = "claude"

[session]
pattern = "hq-operator"
work_dir = "{town}/{rig}"
`))
	if err == nil || !strings.Contains(err.Error(), "town-scoped roles cannot include {rig}") {
		t.Fatalf("ParseRoleSpec() error = %v, want town scope validation", err)
	}
}

func TestParseRoleSpec_RequiresRigPlaceholderForRigScope(t *testing.T) {
	_, err := ParseRoleSpec([]byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "codex"

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/crew/{name}"
`))
	if err == nil || !strings.Contains(err.Error(), "{rig}") {
		t.Fatalf("ParseRoleSpec() error = %v, want missing {rig} validation", err)
	}
}

func TestParseRoleSpec_RejectsImpossibleCapabilityOverride(t *testing.T) {
	_, err := ParseRoleSpec([]byte(`
version = 1
role = "reviewer"
scope = "rig"
provider = "codex"

[session]
pattern = "{prefix}-reviewer-{name}"
work_dir = "{town}/{rig}/crew/{name}"

[capabilities]
hooks = true
`))
	if err == nil || !strings.Contains(err.Error(), "does not support hooks") {
		t.Fatalf("ParseRoleSpec() error = %v, want hooks validation", err)
	}
}

func TestParseRoleSpec_AllowsExplicitSupportedOverrides(t *testing.T) {
	spec, err := ParseRoleSpec([]byte(`
version = 1
role = "mayor_assistant"
scope = "town"
provider = "claude"

[session]
pattern = "hq-mayor-assistant"
work_dir = "{town}"
start_command = "exec claude --continue"

[capabilities]
resume = true
fork_session = true
ready_strategy = "prompt"

[env]
CUSTOM_FLAG = "enabled"
`))
	if err != nil {
		t.Fatalf("ParseRoleSpec() error = %v", err)
	}

	if spec.Session.StartCommand != "exec claude --continue" {
		t.Fatalf("StartCommand = %q, want explicit override preserved", spec.Session.StartCommand)
	}
	if spec.Env["CUSTOM_FLAG"] != "enabled" {
		t.Fatalf("CUSTOM_FLAG = %q, want enabled", spec.Env["CUSTOM_FLAG"])
	}
	if !spec.Capabilities.ForkSession {
		t.Fatalf("ForkSession = false, want true")
	}
	if spec.Capabilities.ReadyStrategy != "prompt" {
		t.Fatalf("ReadyStrategy = %q, want prompt", spec.Capabilities.ReadyStrategy)
	}
}

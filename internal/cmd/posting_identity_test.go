package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/posting"
)

func TestGetAgentIdentity_PolecatWithPosting(t *testing.T) {
	dir := t.TempDir()
	if err := posting.Write(dir, "scout"); err != nil {
		t.Fatal(err)
	}
	ctx := RoleContext{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "furiosa",
		WorkDir: dir,
	}
	got := getAgentIdentity(ctx)
	want := "gastown/polecats/furiosa[scout]"
	if got != want {
		t.Errorf("getAgentIdentity() = %q, want %q", got, want)
	}
}

func TestGetAgentIdentity_PolecatNoPosting(t *testing.T) {
	dir := t.TempDir()
	ctx := RoleContext{
		Role:    RolePolecat,
		Rig:     "gastown",
		Polecat: "furiosa",
		WorkDir: dir,
	}
	got := getAgentIdentity(ctx)
	want := "gastown/polecats/furiosa"
	if got != want {
		t.Errorf("getAgentIdentity() = %q, want %q", got, want)
	}
}

func TestGetAgentIdentity_CrewWithPosting(t *testing.T) {
	dir := t.TempDir()
	if err := posting.Write(dir, "dispatcher"); err != nil {
		t.Fatal(err)
	}
	ctx := RoleContext{
		Role:    RoleCrew,
		Rig:     "gastown",
		Polecat: "diesel",
		WorkDir: dir,
	}
	got := getAgentIdentity(ctx)
	want := "gastown/crew/diesel[dispatcher]"
	if got != want {
		t.Errorf("getAgentIdentity() = %q, want %q", got, want)
	}
}

func TestGetAgentIdentity_UsesPostingField(t *testing.T) {
	// When Posting is set on context, it should be used without reading file
	ctx := RoleContext{
		Role:    RolePolecat,
		Rig:     "myrig",
		Polecat: "toast",
		Posting: "lead",
	}
	got := getAgentIdentity(ctx)
	want := "myrig/polecats/toast[lead]"
	if got != want {
		t.Errorf("getAgentIdentity() = %q, want %q", got, want)
	}
}

func TestGetAgentIdentity_MayorIgnoresPosting(t *testing.T) {
	// Mayor should not include posting
	ctx := RoleContext{
		Role:    RoleMayor,
		Posting: "should-not-appear",
	}
	got := getAgentIdentity(ctx)
	want := "mayor"
	if got != want {
		t.Errorf("getAgentIdentity() = %q, want %q", got, want)
	}
}

func TestDetectSenderFromRole_PolecatWithPosting(t *testing.T) {
	// Set up env vars
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_POLECAT", "furiosa")
	t.Setenv("GT_POSTING", "scout")

	got := detectSenderFromRole("polecat")
	want := "gastown/furiosa[scout]"
	if got != want {
		t.Errorf("detectSenderFromRole() = %q, want %q", got, want)
	}
}

func TestDetectSenderFromRole_CrewWithPosting(t *testing.T) {
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_CREW", "diesel")
	t.Setenv("GT_POSTING", "dispatcher")

	got := detectSenderFromRole("crew")
	want := "gastown/crew/diesel[dispatcher]"
	if got != want {
		t.Errorf("detectSenderFromRole() = %q, want %q", got, want)
	}
}

func TestDetectSenderFromRole_PolecatNoPosting(t *testing.T) {
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_POLECAT", "furiosa")
	os.Unsetenv("GT_POSTING")

	// Create a temp dir as cwd with no .runtime/posting
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := detectSenderFromRole("polecat")
	want := "gastown/furiosa"
	if got != want {
		t.Errorf("detectSenderFromRole() = %q, want %q", got, want)
	}
}

func TestDetectSenderFromRole_FullAddressWithPosting(t *testing.T) {
	t.Setenv("GT_POSTING", "scout")

	got := detectSenderFromRole("gastown/polecats/furiosa")
	want := "gastown/polecats/furiosa[scout]"
	if got != want {
		t.Errorf("detectSenderFromRole() = %q, want %q", got, want)
	}
}

func TestDetectSenderFromRole_WitnessIgnoresPosting(t *testing.T) {
	t.Setenv("GT_RIG", "gastown")
	t.Setenv("GT_POSTING", "should-not-appear")

	got := detectSenderFromRole("witness")
	want := "gastown/witness"
	if got != want {
		t.Errorf("detectSenderFromRole() = %q, want %q", got, want)
	}
}

func TestDetectPosting_FromCwdFile(t *testing.T) {
	dir := t.TempDir()
	if err := posting.Write(dir, "scout"); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("GT_POSTING")
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := detectPosting()
	want := "scout"
	if got != want {
		t.Errorf("detectPosting() = %q, want %q", got, want)
	}
}

func TestDetectPosting_EnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, ".runtime")
	os.MkdirAll(runtimeDir, 0755)
	os.WriteFile(filepath.Join(runtimeDir, "posting"), []byte("file-posting\n"), 0644)

	t.Setenv("GT_POSTING", "env-posting")
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	got := detectPosting()
	want := "env-posting"
	if got != want {
		t.Errorf("detectPosting() = %q, want %q (env should override file)", got, want)
	}
}

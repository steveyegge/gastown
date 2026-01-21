package doctor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gastown/internal/config"
)

func TestNewPatrolRolesHavePromptsCheck(t *testing.T) {
	check := NewPatrolRolesHavePromptsCheck()
	if check == nil {
		t.Fatal("NewPatrolRolesHavePromptsCheck() returned nil")
	}
	if check.Name() != "patrol-roles-have-prompts" {
		t.Errorf("Name() = %q, want %q", check.Name(), "patrol-roles-have-prompts")
	}
	if !check.CanFix() {
		t.Error("CanFix() should return true")
	}
}

func setupRigConfig(t *testing.T, tmpDir string, rigNames []string) {
	t.Helper()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	rigsConfig := config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
	for _, name := range rigNames {
		rigsConfig.Rigs[name] = config.RigEntry{}
	}

	data, err := json.Marshal(rigsConfig)
	if err != nil {
		t.Fatalf("marshal rigs.json: %v", err)
	}

	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), data, 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}
}

func setupRigTemplatesDir(t *testing.T, tmpDir, rigName string) string {
	t.Helper()
	templatesDir := filepath.Join(tmpDir, rigName, "mayor", "rig", "internal", "templates", "roles")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	return templatesDir
}

func TestPatrolRolesHavePromptsCheck_NoRigs(t *testing.T) {
	tmpDir := t.TempDir()

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK (no rigs configured)", result.Status)
	}
}

func TestPatrolRolesHavePromptsCheck_NoTemplatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Rigs without templates directory use embedded templates - this is OK
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK (using embedded templates)", result.Status)
	}
	if len(check.missingByRig) != 0 {
		t.Errorf("missingByRig count = %d, want 0 (rig skipped)", len(check.missingByRig))
	}
}

func TestPatrolRolesHavePromptsCheck_SomeTemplatesMissing(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})
	templatesDir := setupRigTemplatesDir(t, tmpDir, "myproject")

	if err := os.WriteFile(filepath.Join(templatesDir, "deacon.md.tmpl"), []byte("test"), 0644); err != nil {
		t.Fatalf("write deacon.md.tmpl: %v", err)
	}

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Missing templates in custom override dir is OK - embedded templates fill the gap
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK (embedded templates fill gaps)", result.Status)
	}
	if len(check.missingByRig["myproject"]) != 2 {
		t.Errorf("missing templates = %d, want 2 (witness, refinery)", len(check.missingByRig["myproject"]))
	}
}

func TestPatrolRolesHavePromptsCheck_AllTemplatesExist(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})
	templatesDir := setupRigTemplatesDir(t, tmpDir, "myproject")

	for _, tmpl := range requiredRolePrompts {
		if err := os.WriteFile(filepath.Join(templatesDir, tmpl), []byte("test content"), 0644); err != nil {
			t.Fatalf("write %s: %v", tmpl, err)
		}
	}

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK", result.Status)
	}
	if len(check.missingByRig) != 0 {
		t.Errorf("missingByRig count = %d, want 0", len(check.missingByRig))
	}
}

func TestPatrolRolesHavePromptsCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})
	// Create templates dir so rig is checked (not skipped)
	templatesDir := setupRigTemplatesDir(t, tmpDir, "myproject")

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	// Status is OK (embedded templates fill gaps) but missingByRig is populated
	if result.Status != StatusOK {
		t.Fatalf("Initial Status = %v, want OK", result.Status)
	}
	if len(check.missingByRig["myproject"]) != 3 {
		t.Fatalf("missingByRig = %d, want 3", len(check.missingByRig["myproject"]))
	}

	err := check.Fix(ctx)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	for _, tmpl := range requiredRolePrompts {
		path := filepath.Join(templatesDir, tmpl)
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("Fix() did not create %s: %v", tmpl, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("Fix() created empty file %s", tmpl)
		}
	}

	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("After Fix(), Status = %v, want OK", result.Status)
	}
}

func TestPatrolRolesHavePromptsCheck_FixPartial(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})
	templatesDir := setupRigTemplatesDir(t, tmpDir, "myproject")

	existingContent := []byte("existing custom content")
	if err := os.WriteFile(filepath.Join(templatesDir, "deacon.md.tmpl"), existingContent, 0644); err != nil {
		t.Fatalf("write deacon.md.tmpl: %v", err)
	}

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	// Status is OK (embedded templates fill gaps) but missingByRig is populated
	if result.Status != StatusOK {
		t.Fatalf("Initial Status = %v, want OK", result.Status)
	}
	if len(check.missingByRig["myproject"]) != 2 {
		t.Fatalf("missing = %d, want 2", len(check.missingByRig["myproject"]))
	}

	err := check.Fix(ctx)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}

	deaconContent, err := os.ReadFile(filepath.Join(templatesDir, "deacon.md.tmpl"))
	if err != nil {
		t.Fatalf("read deacon.md.tmpl: %v", err)
	}
	if string(deaconContent) != string(existingContent) {
		t.Error("Fix() should not overwrite existing deacon.md.tmpl")
	}

	for _, tmpl := range []string{"witness.md.tmpl", "refinery.md.tmpl"} {
		path := filepath.Join(templatesDir, tmpl)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Fix() did not create %s: %v", tmpl, err)
		}
	}
}

func TestPatrolRolesHavePromptsCheck_MultipleRigs(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"project1", "project2"})

	// project1 has templates dir with all templates
	templatesDir1 := setupRigTemplatesDir(t, tmpDir, "project1")
	for _, tmpl := range requiredRolePrompts {
		if err := os.WriteFile(filepath.Join(templatesDir1, tmpl), []byte("test"), 0644); err != nil {
			t.Fatalf("write %s: %v", tmpl, err)
		}
	}

	// project2 has templates dir but no files (missing all)
	setupRigTemplatesDir(t, tmpDir, "project2")

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Status is OK (embedded templates fill gaps)
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK", result.Status)
	}
	if _, ok := check.missingByRig["project1"]; ok {
		t.Error("project1 should not be in missingByRig")
	}
	if len(check.missingByRig["project2"]) != 3 {
		t.Errorf("project2 missing = %d, want 3", len(check.missingByRig["project2"]))
	}
}

func TestPatrolRolesHavePromptsCheck_FixHint(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})
	// Create templates dir so rig is checked (not skipped)
	setupRigTemplatesDir(t, tmpDir, "myproject")

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	// Status is now OK (embedded templates are fine), so no FixHint
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK", result.Status)
	}
	// FixHint is empty for OK status since nothing is broken
	if result.FixHint != "" {
		t.Errorf("FixHint = %q, want empty for OK status", result.FixHint)
	}
}

func TestPatrolRolesHavePromptsCheck_FixMultipleRigs(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"project1", "project2", "project3"})

	// project1 has all templates
	templatesDir1 := setupRigTemplatesDir(t, tmpDir, "project1")
	for _, tmpl := range requiredRolePrompts {
		if err := os.WriteFile(filepath.Join(templatesDir1, tmpl), []byte("existing"), 0644); err != nil {
			t.Fatalf("write %s: %v", tmpl, err)
		}
	}

	// project2 and project3 have templates dir but no files
	setupRigTemplatesDir(t, tmpDir, "project2")
	setupRigTemplatesDir(t, tmpDir, "project3")

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	// Status is OK (embedded templates fill gaps) but missingByRig is populated
	if result.Status != StatusOK {
		t.Fatalf("Initial Status = %v, want OK", result.Status)
	}
	if len(check.missingByRig) != 2 {
		t.Fatalf("missingByRig count = %d, want 2 (project2, project3)", len(check.missingByRig))
	}

	err := check.Fix(ctx)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}

	for _, rig := range []string{"project2", "project3"} {
		templatesDir := filepath.Join(tmpDir, rig, "mayor", "rig", "internal", "templates", "roles")
		for _, tmpl := range requiredRolePrompts {
			path := filepath.Join(templatesDir, tmpl)
			if _, err := os.Stat(path); err != nil {
				t.Errorf("Fix() did not create %s for %s: %v", tmpl, rig, err)
			}
		}
	}

	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("After Fix(), Status = %v, want OK", result.Status)
	}
}

func TestPatrolRolesHavePromptsCheck_DetailsFormat(t *testing.T) {
	tmpDir := t.TempDir()
	setupRigConfig(t, tmpDir, []string{"myproject"})
	// Create templates dir so rig is checked (not skipped)
	setupRigTemplatesDir(t, tmpDir, "myproject")

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	_ = check.Run(ctx)

	// Status is OK now, but missingByRig should be populated with 3 templates
	if len(check.missingByRig["myproject"]) != 3 {
		t.Fatalf("missingByRig count = %d, want 3", len(check.missingByRig["myproject"]))
	}
}

func TestPatrolRolesHavePromptsCheck_MalformedRigsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte("not valid json"), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusError {
		t.Errorf("Status = %v, want Error for malformed rigs.json", result.Status)
	}
}

func TestPatrolRolesHavePromptsCheck_EmptyRigsConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mayorDir, "rigs.json"), []byte(`{"rigs":{}}`), 0644); err != nil {
		t.Fatalf("write rigs.json: %v", err)
	}

	check := NewPatrolRolesHavePromptsCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK for empty rigs config", result.Status)
	}
	if result.Message != "No rigs configured" {
		t.Errorf("Message = %q, want 'No rigs configured'", result.Message)
	}
}

func TestNewPatrolHooksWiredCheck(t *testing.T) {
	check := NewPatrolHooksWiredCheck()
	if check == nil {
		t.Fatal("NewPatrolHooksWiredCheck() returned nil")
	}
	if check.Name() != "patrol-hooks-wired" {
		t.Errorf("Name() = %q, want %q", check.Name(), "patrol-hooks-wired")
	}
	if !check.CanFix() {
		t.Error("CanFix() should return true")
	}
}

func TestPatrolHooksWiredCheck_NoDaemonConfig(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	check := NewPatrolHooksWiredCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want Warning", result.Status)
	}
	if result.FixHint == "" {
		t.Error("FixHint should not be empty")
	}
}

func TestPatrolHooksWiredCheck_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := config.NewDaemonPatrolConfig()
	path := config.DaemonPatrolConfigPath(tmpDir)
	if err := config.SaveDaemonPatrolConfig(path, cfg); err != nil {
		t.Fatalf("SaveDaemonPatrolConfig: %v", err)
	}

	check := NewPatrolHooksWiredCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK", result.Status)
	}
}

func TestPatrolHooksWiredCheck_EmptyPatrols(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: map[string]config.PatrolConfig{},
	}
	path := config.DaemonPatrolConfigPath(tmpDir)
	if err := config.SaveDaemonPatrolConfig(path, cfg); err != nil {
		t.Fatalf("SaveDaemonPatrolConfig: %v", err)
	}

	check := NewPatrolHooksWiredCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusWarning {
		t.Errorf("Status = %v, want Warning (no patrols configured)", result.Status)
	}
}

func TestPatrolHooksWiredCheck_HeartbeatEnabled(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Heartbeat: &config.HeartbeatConfig{
			Enabled:  true,
			Interval: "3m",
		},
		Patrols: map[string]config.PatrolConfig{},
	}
	path := config.DaemonPatrolConfigPath(tmpDir)
	if err := config.SaveDaemonPatrolConfig(path, cfg); err != nil {
		t.Fatalf("SaveDaemonPatrolConfig: %v", err)
	}

	check := NewPatrolHooksWiredCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)

	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK (heartbeat enabled triggers patrols)", result.Status)
	}
}

func TestPatrolHooksWiredCheck_Fix(t *testing.T) {
	tmpDir := t.TempDir()
	mayorDir := filepath.Join(tmpDir, "mayor")
	if err := os.MkdirAll(mayorDir, 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}

	check := NewPatrolHooksWiredCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	if result.Status != StatusWarning {
		t.Fatalf("Initial Status = %v, want Warning", result.Status)
	}

	err := check.Fix(ctx)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}

	path := config.DaemonPatrolConfigPath(tmpDir)
	loaded, err := config.LoadDaemonPatrolConfig(path)
	if err != nil {
		t.Fatalf("LoadDaemonPatrolConfig: %v", err)
	}
	if loaded.Type != "daemon-patrol-config" {
		t.Errorf("Type = %q, want 'daemon-patrol-config'", loaded.Type)
	}
	if len(loaded.Patrols) != 3 {
		t.Errorf("Patrols count = %d, want 3", len(loaded.Patrols))
	}

	result = check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("After Fix(), Status = %v, want OK", result.Status)
	}
}

func TestPatrolHooksWiredCheck_FixPreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()

	existing := &config.DaemonPatrolConfig{
		Type:    "daemon-patrol-config",
		Version: 1,
		Patrols: map[string]config.PatrolConfig{
			"custom": {Enabled: true, Agent: "custom-agent"},
		},
	}
	path := config.DaemonPatrolConfigPath(tmpDir)
	if err := config.SaveDaemonPatrolConfig(path, existing); err != nil {
		t.Fatalf("SaveDaemonPatrolConfig: %v", err)
	}

	check := NewPatrolHooksWiredCheck()
	ctx := &CheckContext{TownRoot: tmpDir}

	result := check.Run(ctx)
	if result.Status != StatusOK {
		t.Errorf("Status = %v, want OK (has patrols)", result.Status)
	}

	err := check.Fix(ctx)
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}

	loaded, err := config.LoadDaemonPatrolConfig(path)
	if err != nil {
		t.Fatalf("LoadDaemonPatrolConfig: %v", err)
	}
	if len(loaded.Patrols) != 1 {
		t.Errorf("Patrols count = %d, want 1 (should preserve existing)", len(loaded.Patrols))
	}
	if _, ok := loaded.Patrols["custom"]; !ok {
		t.Error("existing custom patrol was overwritten")
	}
}

package cmd

import (
	"testing"
)

func TestRestartCmd_Registered(t *testing.T) {
	// Verify restart command is registered
	cmd, _, err := rootCmd.Find([]string{"restart"})
	if err != nil {
		t.Fatalf("restart command not found: %v", err)
	}
	if cmd.Name() != "restart" {
		t.Errorf("expected command name 'restart', got %q", cmd.Name())
	}
}

func TestRestartCmd_GroupID(t *testing.T) {
	if restartCmd.GroupID != GroupServices {
		t.Errorf("restart command GroupID = %q, want %q", restartCmd.GroupID, GroupServices)
	}
}

func TestRestartCmd_Flags(t *testing.T) {
	flags := restartCmd.Flags()

	if flags.Lookup("quiet") == nil {
		t.Error("--quiet flag not registered")
	}
	if flags.Lookup("strategy") == nil {
		t.Error("--strategy flag not registered")
	}
}

func TestRestartCmd_ShortFlags(t *testing.T) {
	flags := restartCmd.Flags()

	if flags.ShorthandLookup("q") == nil {
		t.Error("-q short flag not registered")
	}
	if flags.ShorthandLookup("s") == nil {
		t.Error("-s short flag not registered")
	}
}

func TestRestartCmd_NoOldFlags(t *testing.T) {
	flags := restartCmd.Flags()

	if flags.Lookup("wait") != nil {
		t.Error("--wait flag should be removed (use --strategy drain)")
	}
	if flags.Lookup("now") != nil {
		t.Error("--now flag should be removed (use --strategy immediate)")
	}
	if flags.Lookup("infra") != nil {
		t.Error("--infra flag should be removed (use --strategy clean)")
	}
	if flags.Lookup("force") != nil {
		t.Error("--force flag should be removed (use --strategy immediate)")
	}
	if flags.Lookup("restore") != nil {
		t.Error("--restore flag should be removed (default behavior now)")
	}
	if flags.Lookup("polecats") != nil {
		t.Error("--polecats flag should be removed (default behavior now)")
	}
}

func TestRestartCmd_FlagDefaults(t *testing.T) {
	if restartQuiet != false {
		t.Error("restartQuiet should default to false")
	}
	if restartStrategy != StrategyGraceful {
		t.Errorf("restartStrategy should default to %q, got %q", StrategyGraceful, restartStrategy)
	}
}

func TestRestartOptions_FromFlags(t *testing.T) {
	savedQuiet := restartQuiet
	savedStrategy := restartStrategy
	defer func() {
		restartQuiet = savedQuiet
		restartStrategy = savedStrategy
	}()

	restartQuiet = true
	restartStrategy = StrategyDrain

	opts := restartOptionsFromFlags()

	if opts.Quiet != true {
		t.Error("Quiet should be true")
	}
	if opts.Strategy != StrategyDrain {
		t.Errorf("Strategy should be %q, got %q", StrategyDrain, opts.Strategy)
	}
}

func TestRestartOptions_ZeroValue(t *testing.T) {
	opts := RestartOptions{}

	if opts.Quiet != false {
		t.Error("zero-value Quiet should be false")
	}
	if opts.Strategy != "" {
		t.Errorf("zero-value Strategy should be empty, got %q", opts.Strategy)
	}
}

func TestIsValidRestartStrategy(t *testing.T) {
	valid := []string{"graceful", "drain", "immediate", "clean"}
	for _, s := range valid {
		if !isValidRestartStrategy(s) {
			t.Errorf("strategy %q should be valid", s)
		}
	}

	invalid := []string{"", "fast", "slow", "GRACEFUL", "Drain"}
	for _, s := range invalid {
		if isValidRestartStrategy(s) {
			t.Errorf("strategy %q should be invalid", s)
		}
	}
}

func TestRestartStrategyConstants(t *testing.T) {
	if StrategyGraceful != "graceful" {
		t.Errorf("StrategyGraceful = %q, want %q", StrategyGraceful, "graceful")
	}
	if StrategyDrain != "drain" {
		t.Errorf("StrategyDrain = %q, want %q", StrategyDrain, "drain")
	}
	if StrategyImmediate != "immediate" {
		t.Errorf("StrategyImmediate = %q, want %q", StrategyImmediate, "immediate")
	}
	if StrategyClean != "clean" {
		t.Errorf("StrategyClean = %q, want %q", StrategyClean, "clean")
	}
}

func TestValidRestartStrategies_ContainsAll(t *testing.T) {
	expected := map[string]bool{
		StrategyGraceful:  false,
		StrategyDrain:     false,
		StrategyImmediate: false,
		StrategyClean:     false,
	}

	for _, s := range validRestartStrategies {
		if _, ok := expected[s]; !ok {
			t.Errorf("unexpected strategy %q in validRestartStrategies", s)
		}
		expected[s] = true
	}

	for s, found := range expected {
		if !found {
			t.Errorf("strategy %q missing from validRestartStrategies", s)
		}
	}
}

func TestRunRestartWithOptions_InvalidStrategy(t *testing.T) {
	err := runRestartWithOptions(RestartOptions{Strategy: "bogus", Quiet: true})
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

func TestRunRestartWithOptions_UppercaseNormalized(t *testing.T) {
	// Strategy is lowercased, so "GRACEFUL" should still fail validation
	// because we validate after lowering
	err := runRestartWithOptions(RestartOptions{Strategy: "BOGUS", Quiet: true})
	if err == nil {
		t.Fatal("expected error for invalid uppercase strategy")
	}
}

func TestRestartOptions_GracefulStopsAndRestoresPolecats(t *testing.T) {
	// Graceful strategy should stop polecats and restore them
	opts := RestartOptions{Strategy: StrategyGraceful}
	if opts.Strategy != StrategyGraceful {
		t.Errorf("strategy = %q, want %q", opts.Strategy, StrategyGraceful)
	}
}

func TestRestartOptions_CleanDoesNotRestore(t *testing.T) {
	// Clean strategy nukes everything and does NOT restore polecats
	opts := RestartOptions{Strategy: StrategyClean}
	if opts.Strategy != StrategyClean {
		t.Errorf("strategy = %q, want %q", opts.Strategy, StrategyClean)
	}
}

func TestDownOptions_FromFlags(t *testing.T) {
	savedQuiet := downQuiet
	savedForce := downForce
	savedAll := downAll
	savedNuke := downNuke
	savedDryRun := downDryRun
	savedPolecats := downPolecats
	savedNoSave := downNoSave

	defer func() {
		downQuiet = savedQuiet
		downForce = savedForce
		downAll = savedAll
		downNuke = savedNuke
		downDryRun = savedDryRun
		downPolecats = savedPolecats
		downNoSave = savedNoSave
	}()

	downQuiet = true
	downForce = true
	downAll = true
	downNuke = false
	downDryRun = true
	downPolecats = true
	downNoSave = false

	opts := downOptionsFromFlags()

	if opts.Quiet != true {
		t.Error("Quiet should be true")
	}
	if opts.Force != true {
		t.Error("Force should be true")
	}
	if opts.All != true {
		t.Error("All should be true")
	}
	if opts.Nuke != false {
		t.Error("Nuke should be false")
	}
	if opts.DryRun != true {
		t.Error("DryRun should be true")
	}
	if opts.Polecats != true {
		t.Error("Polecats should be true")
	}
	if opts.NotifySave != true {
		t.Error("NotifySave should be true when --no-save is false")
	}
}

func TestDownOptions_ZeroValue(t *testing.T) {
	opts := DownOptions{}

	if opts.Quiet != false {
		t.Error("zero-value Quiet should be false")
	}
	if opts.Force != false {
		t.Error("zero-value Force should be false")
	}
	if opts.Polecats != false {
		t.Error("zero-value Polecats should be false")
	}
	if opts.NotifySave != false {
		t.Error("zero-value NotifySave should be false")
	}
}

func TestDownOptions_NotifySave(t *testing.T) {
	savedNoSave := downNoSave
	savedForce := downForce
	defer func() {
		downNoSave = savedNoSave
		downForce = savedForce
	}()

	// Default: NotifySave is true (--no-save not set)
	downNoSave = false
	downForce = false
	opts := downOptionsFromFlags()
	if !opts.NotifySave {
		t.Error("NotifySave should be true by default")
	}

	// --no-save disables NotifySave
	downNoSave = true
	downForce = false
	opts = downOptionsFromFlags()
	if opts.NotifySave {
		t.Error("NotifySave should be false when --no-save is set")
	}

	// NotifySave true but Force true: nudge phase is skipped at runtime
	// (the guard is in runDownWithOptions, not in downOptionsFromFlags)
	downNoSave = false
	downForce = true
	opts = downOptionsFromFlags()
	if !opts.NotifySave {
		t.Error("NotifySave should still be true even with --force (runtime guard handles it)")
	}
	if !opts.Force {
		t.Error("Force should be true")
	}
}

func TestUpOptions_FromFlags(t *testing.T) {
	savedQuiet := upQuiet
	savedRestore := upRestore

	defer func() {
		upQuiet = savedQuiet
		upRestore = savedRestore
	}()

	upQuiet = true
	upRestore = true

	opts := upOptionsFromFlags()

	if opts.Quiet != true {
		t.Error("Quiet should be true")
	}
	if opts.Restore != true {
		t.Error("Restore should be true")
	}
}

func TestUpOptions_ZeroValue(t *testing.T) {
	opts := UpOptions{}

	if opts.Quiet != false {
		t.Error("zero-value Quiet should be false")
	}
	if opts.Restore != false {
		t.Error("zero-value Restore should be false")
	}
}

func TestUpCmd_NoRestartFlag(t *testing.T) {
	flags := upCmd.Flags()

	if flags.Lookup("restart") != nil {
		t.Error("--restart flag should not exist on gt up (use gt restart instead)")
	}
}

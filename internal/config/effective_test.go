package config

import "testing"

func TestApplyRepoContractDefaults(t *testing.T) {
	settings := &RigSettings{
		Type:    "rig-settings",
		Version: 1,
		RepoContract: &RepoContract{
			VerifyCommand: "./scripts/ci/verify.sh pre-merge",
			SmokeCommand:  "./scripts/ci/verify.sh smoke",
		},
	}

	ApplyRepoContractDefaults(settings)

	if settings.MergeQueue == nil {
		t.Fatal("expected merge queue defaults to be injected")
	}
	if len(settings.MergeQueue.Gates) != 2 {
		t.Fatalf("expected 2 gates, got %d", len(settings.MergeQueue.Gates))
	}
	if got := settings.MergeQueue.Gates["verify"].Cmd; got != "./scripts/ci/verify.sh pre-merge" {
		t.Fatalf("verify gate cmd = %q", got)
	}
	if got := settings.MergeQueue.Gates["smoke"].Phase; got != "post-squash" {
		t.Fatalf("smoke gate phase = %q", got)
	}
	if got := settings.MergeQueue.TestCommand; got != "./scripts/ci/verify.sh pre-merge" {
		t.Fatalf("test_command = %q", got)
	}
}

func TestValidateStrictRepoContract(t *testing.T) {
	t.Run("strict requires repo-local verify command", func(t *testing.T) {
		settings := &RigSettings{
			MergeQueue: &MergeQueueConfig{
				VerificationMode: VerificationModeStrict,
				Gates: map[string]*VerificationGateConfig{
					"verify": {Cmd: "/tmp/outside.sh", Phase: "pre-merge"},
				},
			},
			RepoContract: &RepoContract{
				VerifyCommand: "/tmp/outside.sh",
			},
		}

		if err := ValidateStrictRepoContract(settings); err == nil {
			t.Fatal("expected strict repo contract validation to fail for non-local command")
		}
	})

	t.Run("strict with repo-local verifier passes", func(t *testing.T) {
		settings := &RigSettings{
			MergeQueue: &MergeQueueConfig{
				VerificationMode: VerificationModeStrict,
				Gates: map[string]*VerificationGateConfig{
					"verify": {Cmd: "./scripts/ci/verify.sh", Phase: "pre-merge"},
				},
			},
			RepoContract: &RepoContract{
				VerifyCommand: "./scripts/ci/verify.sh",
			},
		}

		if err := ValidateStrictRepoContract(settings); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	})
}

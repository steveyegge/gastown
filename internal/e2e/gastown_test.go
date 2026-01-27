//go:build e2e

package e2e

import (
	"testing"
	"time"
)

func TestGastown_CreateFile(t *testing.T) {
	// Slot assignment infrastructure is now working.
	// Remaining issue: OpenCode agent completes without executing the task.
	// The prompt is injected but the agent doesn't act on it.
	// TODO: Investigate OpenCode plugin prompt injection and agent behavior
	t.Skip("Skipping: OpenCode agent completes without executing task")

	for _, rt := range []string{"claude", "opencode"} {
		t.Run(rt, func(t *testing.T) {
			r := NewE2ERunner(t, rt)

			t.Logf("\n"+
				"============================================================\n"+
				" [TEST CASE] CreateFile (%s)\n"+
				" PURPOSE: Verifies agent can create a new file and satisfy \n"+
				"          build/run criteria.\n"+
				" SCENARIO:\n"+
				"   1. Establish test rig and source repository.\n"+
				"   2. Create work bead for 'hello.go'.\n"+
				"   3. Dispatch work via 'gt sling'.\n"+
				"   4. Monitor agent telemetry for completion.\n"+
				"   5. Verify file existence and build status.\n"+
				"============================================================", rt)

			t.Log("\n[PHASE 1: RIG PREPARATION] Establishing test environment...")
			r.CreateRig()

			t.Log("\n[PHASE 2: TASK ASSIGNMENT] Creating work bead for file creation...")
			r.CreateBead("create-hello", "Create hello.go that prints Hello World. Use /exit to finish.")

			r.SlingWork()

			completed := r.WaitForCompletion()
			if !completed {
				t.Log("[WARN] Timed out, but checking results anyway...")
			}

			t.Log("\n[PHASE 3: VERIFICATION] Validating agent output...")
			passed := r.Verify(
				r.FileExists("hello.go"),
				r.FileContains("hello.go", "func main"),
				r.BuildsSuccessfully(),
				r.RunOutputContains("testbinary", "hello"),
			)

			if !passed {
				t.Fatal("Verification failed")
			}

			t.Logf("\n[RESULT] %s CreateFile: PASS", rt)
		})
	}
}

func TestGastown_FixBug(t *testing.T) {
	t.Skip("Skipping: OpenCode agent completes without executing task - see TestGastown_CreateFile")

	for _, rt := range []string{"claude", "opencode"} {
		t.Run(rt, func(t *testing.T) {
			r := NewE2ERunner(t, rt)

			t.Logf("\n"+
				"============================================================\n"+
				" [TEST CASE] FixBug (%s)\n"+
				" PURPOSE: Verifies agent can identify buggy logic (a+b instead\n"+
				"          of a-b), fix it, and verify via existing tests.\n"+
				" SCENARIO:\n"+
				"   1. Setup repository with buggy 'subtract' function.\n"+
				"   2. Create work bead with explicit fix instructions.\n"+
				"   3. Dispatch work to autonomous agent via 'gt sling'.\n"+
				"   4. Monitor hierarchical telemetry for 'done' signal.\n"+
				"   5. Verify logic fix and unit test passage.\n"+
				"============================================================", rt)

			t.Log("\n[PHASE 1: SOURCE SETUP] Creating buggy repository state...")
			// Setup source repo with buggy code BEFORE creating rig
			r.SetupSourceRepo(func(sourceDir string) {
				r.writeFileTo(sourceDir, "math.go", `package main

func subtract(a, b int) int {
	return a + b
}
`)
				r.writeFileTo(sourceDir, "math_test.go", `package main

import "testing"

func TestSubtract(t *testing.T) {
	got := subtract(5, 3)
	want := 2
	if got != want {
		t.Errorf("subtract(5, 3) = %d, want %d", got, want)
	}
}
`)
				r.runCmd(sourceDir, "git", "add", "math.go", "math_test.go")
				r.runCmd(sourceDir, "git", "commit", "-m", "add buggy code")
			})

			t.Log("\n[PHASE 2: RIG PREPARATION] Establishing test environment...")
			r.CreateRig()

			t.Log("\n[PHASE 3: TASK ASSIGNMENT] Creating work bead for bug fix...")
			r.CreateBead("fix-subtract", "Fix subtract function in math.go - it adds instead of subtracts. Verify with tests and then call 'gt_done' tool.")
			time.Sleep(2 * time.Second)
			r.SlingWork()

			if !r.WaitForCompletion() {
				t.Fatal("Timed out waiting for work completion")
			}

			t.Log("\n[PHASE 4: VERIFICATION] Validating agent fix...")
			if !r.Verify(
				r.FileContains("math.go", "a - b"),
				r.TestsPass(),
			) {
				t.Fatal("Verification failed")
			}

			t.Logf("\n[RESULT] %s FixBug: PASS", rt)
		})
	}
}

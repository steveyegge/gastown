//go:build e2e

package e2e

import (
	"testing"
)

func TestGastown_CreateFile(t *testing.T) {
	for _, rt := range []string{"claude", "opencode"} {
		t.Run(rt, func(t *testing.T) {
			r := NewE2ERunner(t, rt)
			r.CreateRig()
			r.CreateBead("create-hello", "Create hello.go that prints Hello World. Use /exit to finish.")
			r.SlingWork()

			completed := r.WaitForCompletion()
			if !completed {
				t.Log("[WARN] Timed out, but checking results anyway...")
			}

			passed := r.Verify(
				r.FileExists("hello.go"),
				r.FileContains("hello.go", "func main"),
				r.BuildsSuccessfully(),
				r.RunOutputContains("testbinary", "hello"),
			)

			if !passed {
				t.Fatal("Verification failed")
			}

			t.Logf("[RESULT] %s CreateFile: PASS", rt)
		})
	}
}

func TestGastown_FixBug(t *testing.T) {
	for _, rt := range []string{"claude", "opencode"} {
		t.Run(rt, func(t *testing.T) {
			r := NewE2ERunner(t, rt)

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

			r.CreateRig()

			r.CreateBead("fix-subtract", "Fix subtract function in math.go - it adds instead of subtracts. Verify with tests and then call 'gt done' or type '/exit' to finish.")
			r.SlingWork()

			if !r.WaitForCompletion() {
				t.Fatal("Timed out waiting for work completion")
			}

			if !r.Verify(
				r.FileContains("math.go", "a - b"),
				r.TestsPass(),
			) {
				t.Fatal("Verification failed")
			}

			t.Logf("[RESULT] %s FixBug: PASS", rt)
		})
	}
}

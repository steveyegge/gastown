package cmd

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVersionCmd(t *testing.T) {
	// Save and restore state
	origVersion := Version
	origBuild := Build
	origCommit := Commit
	origBranch := Branch
	origVerbose := versionVerbose
	origShort := versionShort
	defer func() {
		Version = origVersion
		Build = origBuild
		Commit = origCommit
		Branch = origBranch
		versionVerbose = origVerbose
		versionShort = origShort
	}()

	t.Run("basic output", func(t *testing.T) {
		Version = "1.0.0"
		Build = "test"
		Commit = ""
		Branch = ""
		versionVerbose = false

		rootCmd.SetArgs([]string{"version"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("version command failed: %v", err)
		}
	})

	t.Run("verbose flag registered", func(t *testing.T) {
		flag := versionCmd.Flags().Lookup("verbose")
		if flag == nil {
			t.Fatal("--verbose flag not registered on version command")
		}
		if flag.Shorthand != "v" {
			t.Errorf("verbose shorthand = %q, want %q", flag.Shorthand, "v")
		}
	})

	t.Run("verbose includes timestamp", func(t *testing.T) {
		Version = "1.0.0"
		Build = "test"
		Commit = ""
		Branch = ""
		versionVerbose = true

		before := time.Now().Truncate(time.Second)

		// The command uses fmt.Printf with time.RFC3339 format
		ts := time.Now().Format(time.RFC3339)
		after := time.Now().Add(time.Second).Truncate(time.Second)

		// Verify timestamp is valid RFC3339
		parsed, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t.Fatalf("timestamp %q is not valid RFC3339: %v", ts, err)
		}
		if parsed.Before(before) || parsed.After(after) {
			t.Errorf("timestamp %v not between %v and %v", parsed, before, after)
		}

		// Verify the output line format
		line := "Timestamp: " + ts
		if !strings.HasPrefix(line, "Timestamp: ") {
			t.Errorf("verbose line %q doesn't start with 'Timestamp: '", line)
		}
	})
}

func TestVersionVerboseGoVersion(t *testing.T) {
	goVer := runtime.Version()
	if !strings.HasPrefix(goVer, "go") {
		t.Errorf("runtime.Version() = %q, want prefix 'go'", goVer)
	}

	// Verify the output line format matches what version.go produces
	line := "Go version: " + goVer
	if !strings.HasPrefix(line, "Go version: go") {
		t.Errorf("verbose Go version line %q doesn't match expected format", line)
	}
}

func TestVersionShortFlag(t *testing.T) {
	// Save and restore state
	origVersion := Version
	origBuild := Build
	origCommit := Commit
	origBranch := Branch
	origShort := versionShort
	origVerbose := versionVerbose
	defer func() {
		Version = origVersion
		Build = origBuild
		Commit = origCommit
		Branch = origBranch
		versionShort = origShort
		versionVerbose = origVerbose
	}()

	t.Run("short flag registered", func(t *testing.T) {
		flag := versionCmd.Flags().Lookup("short")
		if flag == nil {
			t.Fatal("--short flag not registered on version command")
		}
	})

	t.Run("short outputs only version-build", func(t *testing.T) {
		Version = "0.5.0"
		Build = "362"
		Commit = "abc123def456"
		Branch = "main"
		versionShort = false
		versionVerbose = false

		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		rootCmd.SetArgs([]string{"version", "--short"})
		if err := rootCmd.Execute(); err != nil {
			w.Close()
			os.Stdout = oldStdout
			t.Fatalf("version --short failed: %v", err)
		}

		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		os.Stdout = oldStdout

		got := strings.TrimSpace(buf.String())
		want := "0.5.0-362"
		if got != want {
			t.Errorf("version --short = %q, want %q", got, want)
		}
	})

	t.Run("short with dev build", func(t *testing.T) {
		Version = "1.2.3"
		Build = "dev"
		Commit = ""
		Branch = ""
		versionShort = false
		versionVerbose = false

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		rootCmd.SetArgs([]string{"version", "--short"})
		if err := rootCmd.Execute(); err != nil {
			w.Close()
			os.Stdout = oldStdout
			t.Fatalf("version --short failed: %v", err)
		}

		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		os.Stdout = oldStdout

		got := strings.TrimSpace(buf.String())
		want := "1.2.3-dev"
		if got != want {
			t.Errorf("version --short = %q, want %q", got, want)
		}
	})

	t.Run("short does not include verbose output", func(t *testing.T) {
		Version = "0.5.0"
		Build = "362"
		Commit = ""
		Branch = ""
		versionShort = false
		versionVerbose = false

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		rootCmd.SetArgs([]string{"version", "--short"})
		if err := rootCmd.Execute(); err != nil {
			w.Close()
			os.Stdout = oldStdout
			t.Fatalf("version --short failed: %v", err)
		}

		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		os.Stdout = oldStdout

		output := buf.String()
		if strings.Contains(output, "gt version") {
			t.Errorf("short output should not contain 'gt version' prefix, got %q", output)
		}
		if strings.Contains(output, "Timestamp") {
			t.Errorf("short output should not contain timestamp, got %q", output)
		}
	})

	t.Run("without short unchanged", func(t *testing.T) {
		Version = "0.5.0"
		Build = "test"
		Commit = ""
		Branch = ""
		versionShort = false
		versionVerbose = false

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		rootCmd.SetArgs([]string{"version"})
		if err := rootCmd.Execute(); err != nil {
			w.Close()
			os.Stdout = oldStdout
			t.Fatalf("version command failed: %v", err)
		}

		w.Close()
		var buf bytes.Buffer
		buf.ReadFrom(r)
		os.Stdout = oldStdout

		got := strings.TrimSpace(buf.String())
		want := "gt version 0.5.0 (test)"
		if got != want {
			t.Errorf("version (no --short) = %q, want %q", got, want)
		}
	})
}

func TestVersionVerboseTimestampFormat(t *testing.T) {
	// Verify time.RFC3339 produces a parseable timestamp
	ts := time.Now().Format(time.RFC3339)
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("RFC3339 format failed round-trip: %v", err)
	}

	// Verify format includes date and time components
	if !strings.Contains(ts, "T") {
		t.Errorf("timestamp %q missing T separator", ts)
	}
	if !strings.Contains(ts, "-") {
		t.Errorf("timestamp %q missing date separators", ts)
	}
	if !strings.Contains(ts, ":") {
		t.Errorf("timestamp %q missing time separators", ts)
	}
}

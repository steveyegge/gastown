package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	stubDir, err := os.MkdirTemp("", "gt-agent-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create stub dir: %v\n", err)
		os.Exit(1)
	}

	stub := []byte("#!/bin/sh\nexit 0\n")
	binaries := []string{
		"claude",
		"gemini",
		"codex",
		"cursor-agent",
		"auggie",
		"amp",
	}
	for _, name := range binaries {
		path := filepath.Join(stubDir, name)
		if err := os.WriteFile(path, stub, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "write stub %s: %v\n", name, err)
			os.Exit(1)
		}
	}

	originalPath := os.Getenv("PATH")
	_ = os.Setenv("PATH", stubDir+string(os.PathListSeparator)+originalPath)

	code := m.Run()

	_ = os.Setenv("PATH", originalPath)
	_ = os.RemoveAll(stubDir)
	os.Exit(code)
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// cmdRegister implements 'faultline register <project-name> [flags]'.
func cmdRegister() {
	// Parse arguments after "register".
	args := os.Args[2:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: faultline register <project-name> [--rig <rig>] [--url <service-url>] [--write-env]")
		os.Exit(1)
	}

	var projectName, rig, serviceURL, token string
	var writeEnv bool

	// First positional arg is the project name.
	projectName = args[0]
	if strings.HasPrefix(projectName, "-") {
		fmt.Fprintln(os.Stderr, "Usage: faultline register <project-name> [--rig <rig>] [--url <service-url>] [--write-env]")
		os.Exit(1)
	}

	// Parse remaining flags.
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--rig":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --rig requires a value")
				os.Exit(1)
			}
			i++
			rig = args[i]
		case "--url":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --url requires a value")
				os.Exit(1)
			}
			i++
			serviceURL = args[i]
		case "--token":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "error: --token requires a value")
				os.Exit(1)
			}
			i++
			token = args[i]
		case "--write-env":
			writeEnv = true
		default:
			fmt.Fprintf(os.Stderr, "unknown flag: %s\n", args[i])
			fmt.Fprintln(os.Stderr, "Usage: faultline register <project-name> [--rig <rig>] [--url <service-url>] [--write-env]")
			os.Exit(1)
		}
	}

	// Detect language from project files.
	language := detectLanguage()

	// Resolve server address and auth token.
	serverAddr := envOr("FAULTLINE_SERVER", envOr("FAULTLINE_ADDR", ":8080"))
	if token == "" {
		token = os.Getenv("FAULTLINE_API_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "error: API token required. Set FAULTLINE_API_TOKEN or use --token")
		os.Exit(1)
	}

	// Build the API URL.
	apiBase := serverAddr
	if !strings.HasPrefix(apiBase, "http") {
		if strings.HasPrefix(apiBase, ":") {
			apiBase = "localhost" + apiBase
		}
		apiBase = "http://" + apiBase
	}
	registerURL := strings.TrimSuffix(apiBase, "/") + "/api/v1/register"

	// Build request payload.
	payload := map[string]string{
		"name": projectName,
	}
	if rig != "" {
		payload["rig"] = rig
	}
	if language != "" {
		payload["language"] = language
	}
	if serviceURL != "" {
		payload["url"] = serviceURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encoding request: %v\n", err)
		os.Exit(1)
	}

	// Call the register API.
	req, err := http.NewRequest("POST", registerURL, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error connecting to faultline server at %s: %v\n", apiBase, err)
		fmt.Fprintln(os.Stderr, "Is the server running? Try: faultline status")
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		fmt.Fprintf(os.Stderr, "registration failed (%d): %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	// Parse response.
	var result struct {
		ProjectID int64             `json:"project_id"`
		Name      string            `json:"name"`
		Rig       string            `json:"rig"`
		PublicKey string            `json:"public_key"`
		DSN       string            `json:"dsn"`
		Endpoints map[string]string `json:"endpoints"`
		EnvVar    string            `json:"env_var"`
		Setup     string            `json:"setup"`
		Notes     []string          `json:"notes"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing response: %v\n", err)
		os.Exit(1)
	}

	// Display results.
	fmt.Println()
	fmt.Printf("✓ Project registered: %s\n", result.Name)
	fmt.Printf("  Project ID: %d\n", result.ProjectID)
	fmt.Printf("  Rig:        %s\n", result.Rig)
	fmt.Println()
	fmt.Printf("  DSN: %s\n", result.DSN)
	fmt.Println()

	// Show setup snippet if available.
	if result.Setup != "" {
		if language != "" {
			fmt.Printf("  Setup (%s detected):\n", language)
		} else {
			fmt.Println("  Setup:")
		}
		fmt.Println()
		for _, line := range strings.Split(result.Setup, "\n") {
			fmt.Printf("    %s\n", line)
		}
		fmt.Println()
	}

	// Show notes.
	if len(result.Notes) > 0 {
		fmt.Println("  Notes:")
		for _, note := range result.Notes {
			fmt.Printf("    • %s\n", note)
		}
		fmt.Println()
	}

	// Optionally write .env file.
	if writeEnv {
		if err := writeEnvFile(result.EnvVar); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write .env: %v\n", err)
		} else {
			fmt.Println("  ✓ Written to .env")
		}
	} else {
		fmt.Printf("  To add to .env:  echo '%s' >> .env\n", result.EnvVar)
		fmt.Println("  Or run with --write-env to do it automatically.")
	}
}

// detectLanguage checks for common project manifest files to determine the language.
func detectLanguage() string {
	checks := []struct {
		file     string
		language string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"requirements.txt", "python"},
		{"pyproject.toml", "python"},
		{"setup.py", "python"},
		{"Podfile", "swift"},
		{"Package.swift", "swift"},
	}
	for _, c := range checks {
		if _, err := os.Stat(c.file); err == nil {
			return c.language
		}
	}
	return ""
}

// writeEnvFile appends the FAULTLINE_DSN line to a .env file, creating it if needed.
func writeEnvFile(envVar string) error {
	const envFile = ".env"

	// If file exists, check if DSN is already set.
	if data, err := os.ReadFile(envFile); err == nil {
		content := string(data)
		if strings.Contains(content, "FAULTLINE_DSN=") {
			return fmt.Errorf("FAULTLINE_DSN already exists in .env — update manually if needed")
		}
		// Append with newline separator if file doesn't end with one.
		if len(content) > 0 && !strings.HasSuffix(content, "\n") {
			envVar = "\n" + envVar
		}
	}

	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	_, err = fmt.Fprintln(f, envVar)
	return err
}

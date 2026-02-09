package cmd

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	yaml "go.yaml.in/yaml/v2"

	"github.com/steveyegge/gastown/internal/workspace"
)

// healthResponse is the JSON response from the daemon health endpoint.
type healthResponse struct {
	Status  string  `json:"status"`
	Uptime  float64 `json:"uptime"`
	Version string  `json:"version"`
}

// newDaemonHTTPClient creates an HTTP client with a 10-second timeout.
// If BD_INSECURE_SKIP_VERIFY is set, TLS certificate verification is skipped.
func newDaemonHTTPClient() *http.Client {
	client := &http.Client{Timeout: 10 * time.Second}
	if os.Getenv("BD_INSECURE_SKIP_VERIFY") != "" {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return client
}

// callHealth makes the Connect-RPC health call and returns the parsed response.
func callHealth(client *http.Client, daemonURL, token string) (*healthResponse, error) {
	url := strings.TrimRight(daemonURL, "/") + "/bd.v1.BeadsService/Health"
	req, err := http.NewRequest("POST", url, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("cannot reach daemon at %s: %v", daemonURL, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach daemon at %s: %v", daemonURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("authentication failed - check your token")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cannot reach daemon at %s: %v", daemonURL, err)
	}

	var health healthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("cannot reach daemon at %s: unexpected response: %v", daemonURL, err)
	}

	if health.Status != "healthy" {
		return nil, fmt.Errorf("daemon at %s is unhealthy: %s", daemonURL, health.Status)
	}

	return &health, nil
}

// verifyConnection checks connectivity to the remote daemon.
func verifyConnection(daemonURL, token string) error {
	client := newDaemonHTTPClient()
	health, err := callHealth(client, daemonURL, token)
	if err != nil {
		return err
	}

	fmt.Printf("Connected to daemon at %s (status: healthy, uptime: %.1fs, version: %s)\n",
		daemonURL, health.Uptime, health.Version)
	return nil
}

// readDaemonConfig reads daemon-host and daemon-token from .beads/config.yaml.
func readDaemonConfig(townRoot string) (daemonHost, daemonToken, configPath string, err error) {
	configPath = filepath.Join(townRoot, ".beads", "config.yaml")

	data, readErr := os.ReadFile(configPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return "", "", configPath, nil
		}
		return "", "", configPath, fmt.Errorf("reading config: %w", readErr)
	}

	if len(data) == 0 {
		return "", "", configPath, nil
	}

	var config yaml.MapSlice
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", "", configPath, fmt.Errorf("parsing config: %w", err)
	}

	for _, item := range config {
		switch item.Key {
		case "daemon-host":
			if v, ok := item.Value.(string); ok {
				daemonHost = v
			}
		case "daemon-token":
			if v, ok := item.Value.(string); ok {
				daemonToken = v
			}
		}
	}

	return daemonHost, daemonToken, configPath, nil
}

// maskToken returns a display-safe representation of a token.
func maskToken(token string) string {
	if token == "" {
		return "(not set)"
	}
	if len(token) > 3 {
		return strings.Repeat("\u2022", 12) + token[len(token)-3:] + " (set)"
	}
	return strings.Repeat("\u2022", 12) + " (set)"
}

// runConnectStatus shows the current remote daemon connection info.
func runConnectStatus(cmd *cobra.Command, args []string) error {
	townRoot, err := workspace.FindFromCwdOrError()
	if err != nil {
		return fmt.Errorf("finding workspace root: %w", err)
	}

	daemonHost, daemonToken, configPath, err := readDaemonConfig(townRoot)
	if err != nil {
		return err
	}

	if daemonHost == "" {
		fmt.Println("Not connected to a remote daemon (using local beads)")
		return nil
	}

	fmt.Println("Remote Daemon Connection")
	fmt.Printf("  URL:     %s\n", daemonHost)
	fmt.Printf("  Token:   %s\n", maskToken(daemonToken))
	fmt.Printf("  Config:  %s\n", configPath)
	fmt.Println()

	client := newDaemonHTTPClient()
	health, err := callHealth(client, daemonHost, daemonToken)
	if err != nil {
		fmt.Printf("  Status:  \u2717 %v\n", err)
	} else {
		fmt.Printf("  Status:  \u2713 healthy (uptime: %.1fs, version: %s)\n",
			health.Uptime, health.Version)
	}

	return nil
}

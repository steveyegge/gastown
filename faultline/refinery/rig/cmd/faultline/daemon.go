package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	runtimeDir  = ".faultline"
	pidFileName = "faultline.pid"
	logFileName = "faultline.log"
)

// runtimePath returns the path to a file in the faultline runtime directory (~/.faultline or ~/gt/.faultline).
func runtimePath(name string) string {
	// Prefer GT town root if available.
	if townRoot := os.Getenv("GT_TOWN_ROOT"); townRoot != "" {
		return filepath.Join(townRoot, runtimeDir, name)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, runtimeDir, name)
}

// ensureRuntimeDir creates the runtime directory if it doesn't exist.
func ensureRuntimeDir() error {
	dir := filepath.Dir(runtimePath(pidFileName))
	return os.MkdirAll(dir, 0o750)
}

// writePID writes the current process PID to the pidfile.
func writePID() error {
	if err := ensureRuntimeDir(); err != nil {
		return err
	}
	return os.WriteFile(runtimePath(pidFileName), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

// readPID reads the PID from the pidfile. Returns 0 if not found.
func readPID() int {
	data, err := os.ReadFile(runtimePath(pidFileName))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// removePID removes the pidfile.
func removePID() {
	_ = os.Remove(runtimePath(pidFileName))
}

// isProcessRunning checks if a process with the given PID is alive.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 checks if process exists without sending a signal.
	return proc.Signal(syscall.Signal(0)) == nil
}

// healthCheck hits the /health endpoint and returns the status.
func healthCheck(addr string) (string, error) {
	if addr == "" {
		addr = ":8080"
	}
	// Normalize address for HTTP client.
	host := addr
	if strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	url := fmt.Sprintf("http://%s/health", host)

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("health check returned %d: %s", resp.StatusCode, body)
	}

	var health struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &health); err != nil {
		return string(body), nil
	}
	return health.Status, nil
}

// cmdStart launches faultline as a background process.
func cmdStart() {
	pid := readPID()
	if pid > 0 && isProcessRunning(pid) {
		fmt.Printf("faultline is already running (PID %d)\n", pid)
		os.Exit(0)
	}

	if err := ensureRuntimeDir(); err != nil {
		fmt.Fprintf(os.Stderr, "error creating runtime dir: %v\n", err)
		os.Exit(1)
	}

	// Open log file for the background process.
	logPath := runtimePath(logFileName)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
		os.Exit(1)
	}

	// Re-exec ourselves with "serve" subcommand.
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding executable: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(exe, "serve") //nolint:gosec // exe is our own binary path
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "FAULTLINE_DAEMON=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting faultline: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("faultline started (PID %d)\n", cmd.Process.Pid)
	fmt.Printf("  Log: %s\n", logPath)
	fmt.Printf("  PID: %s\n", runtimePath(pidFileName))

	// Wait briefly and verify it's healthy.
	addr := envOr("FAULTLINE_ADDR", ":8080")
	time.Sleep(2 * time.Second)
	if status, err := healthCheck(addr); err == nil {
		fmt.Printf("  Health: %s\n", status)
	} else {
		fmt.Printf("  Health: starting (check again in a few seconds)\n")
	}
}

// cmdStop sends SIGTERM to the running faultline process.
func cmdStop() {
	pid := readPID()
	if pid <= 0 {
		fmt.Println("faultline is not running (no pidfile)")
		os.Exit(0)
	}

	if !isProcessRunning(pid) {
		fmt.Printf("faultline is not running (stale pidfile, PID %d)\n", pid)
		removePID()
		os.Exit(0)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error finding process %d: %v\n", pid, err)
		os.Exit(1)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(os.Stderr, "error stopping faultline (PID %d): %v\n", pid, err)
		os.Exit(1)
	}

	// Wait for process to exit (up to 15s).
	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		if !isProcessRunning(pid) {
			removePID()
			fmt.Printf("faultline stopped (was PID %d)\n", pid)
			return
		}
	}

	fmt.Fprintf(os.Stderr, "faultline did not stop within 15s (PID %d), sending SIGKILL\n", pid)
	_ = proc.Signal(syscall.SIGKILL)
	removePID()
}

// cmdStatus prints the current status of the faultline service.
func cmdStatus() {
	pid := readPID()
	addr := envOr("FAULTLINE_ADDR", ":8080")

	if pid <= 0 || !isProcessRunning(pid) {
		// Check if server is running without pidfile (e.g. started manually).
		if status, err := healthCheck(addr); err == nil {
			fmt.Printf("faultline is running (no pidfile, health: %s)\n", status)
			fmt.Printf("  Address: %s\n", addr)
			os.Exit(0)
		}
		fmt.Println("faultline is not running")
		if pid > 0 {
			fmt.Printf("  Stale pidfile (PID %d)\n", pid)
			removePID()
		}
		os.Exit(1)
	}

	fmt.Printf("faultline is running (PID %d)\n", pid)
	fmt.Printf("  Address: %s\n", addr)
	fmt.Printf("  PID file: %s\n", runtimePath(pidFileName))
	fmt.Printf("  Log file: %s\n", runtimePath(logFileName))

	if status, err := healthCheck(addr); err == nil {
		fmt.Printf("  Health: %s\n", status)
	} else {
		fmt.Printf("  Health: unreachable (%v)\n", err)
	}
}

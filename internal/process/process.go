// Package process provides the Processes interface for managing agent processes.
package process

// ID identifies a process managed by the Processes interface.
// This is an opaque identifier returned by Start() and passed to
// other methods to specify which process to operate on.
type ID string

// Processes manages agent processes across sessions.
// It provides runtime-aware lifecycle management on top of session.Sessions,
// handling startup sequences, readiness detection, and graceful shutdown.
//
// This is a collection interface where methods take an ID
// to specify which process to operate on.
type Processes interface {
	// Start launches an agent process in a new session.
	// Takes a name and returns the process ID.
	// Returns error if already running or fails to start.
	Start(name, workDir, command string) (ID, error)

	// Stop terminates an agent process.
	// If graceful is true, attempts clean shutdown (Ctrl-C) first.
	Stop(id ID, graceful bool) error

	// Restart stops and restarts an agent with a new command.
	Restart(id ID, command string) error

	// IsAlive checks if an agent is running.
	// Returns true if session exists AND agent process is running.
	IsAlive(id ID) bool

	// Send sends text to an agent (waits for readiness on first send).
	Send(id ID, text string) error

	// SendControl sends a control sequence to an agent.
	SendControl(id ID, key string) error

	// Capture returns the last N lines from an agent's session.
	Capture(id ID, lines int) (string, error)
}

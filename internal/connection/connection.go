// Package connection provides an abstraction for local and remote operations.
// This allows Gas Town to manage rigs on remote machines via SSH using
// the same interface as local operations.
package connection

import (
	"io/fs"
	"time"
)

// Connection abstracts file operations, command execution, and tmux management
// for both local and remote (SSH) execution contexts.
type Connection interface {
	// Identification

	// Name returns a human-readable name for this connection.
	Name() string

	// IsLocal returns true if this is a local connection.
	IsLocal() bool

	// File operations

	// ReadFile reads the named file and returns its contents.
	ReadFile(path string) ([]byte, error)

	// WriteFile writes data to the named file with the given permissions.
	WriteFile(path string, data []byte, perm fs.FileMode) error

	// MkdirAll creates a directory and all parent directories.
	MkdirAll(path string, perm fs.FileMode) error

	// Remove removes the named file or empty directory.
	Remove(path string) error

	// RemoveAll removes the named file or directory and any children.
	RemoveAll(path string) error

	// Stat returns file info for the named file.
	Stat(path string) (FileInfo, error)

	// Glob returns the names of all files matching the pattern.
	Glob(pattern string) ([]string, error)

	// Exists returns true if the path exists.
	Exists(path string) (bool, error)

	// Command execution

	// Exec runs a command and returns its combined output.
	Exec(cmd string, args ...string) ([]byte, error)

	// ExecDir runs a command in the specified directory.
	ExecDir(dir, cmd string, args ...string) ([]byte, error)

	// ExecEnv runs a command with additional environment variables.
	ExecEnv(env map[string]string, cmd string, args ...string) ([]byte, error)

	// Tmux operations

	// TmuxNewSession creates a new tmux session with the given name.
	TmuxNewSession(name, dir string) error

	// TmuxKillSession terminates the named tmux session.
	// Uses KillSessionWithProcesses internally to ensure all descendant processes are killed.
	TmuxKillSession(name string) error

	// TmuxSendKeys sends keys to the named tmux session.
	TmuxSendKeys(session, keys string) error

	// TmuxCapturePane captures the last N lines from a tmux pane.
	TmuxCapturePane(session string, lines int) (string, error)

	// TmuxHasSession returns true if the named tmux session exists.
	TmuxHasSession(name string) (bool, error)

	// TmuxListSessions returns a list of all tmux session names.
	TmuxListSessions() ([]string, error)
}

// FileInfo abstracts fs.FileInfo for use over remote connections.
// This is needed because fs.FileInfo contains methods that can't be
// easily serialized over SSH.
type FileInfo interface {
	// Name returns the base name of the file.
	Name() string

	// Size returns the length in bytes.
	Size() int64

	// Mode returns the file mode bits.
	Mode() fs.FileMode

	// ModTime returns the modification time.
	ModTime() time.Time

	// IsDir returns true if this is a directory.
	IsDir() bool
}

// BasicFileInfo is a simple implementation of FileInfo.
type BasicFileInfo struct {
	FileName    string      `json:"name"`
	FileSize    int64       `json:"size"`
	FileMode    fs.FileMode `json:"mode"`
	FileModTime time.Time   `json:"mod_time"`
	FileIsDir   bool        `json:"is_dir"`
}

// Name implements FileInfo.
func (f BasicFileInfo) Name() string { return f.FileName }

// Size implements FileInfo.
func (f BasicFileInfo) Size() int64 { return f.FileSize }

// Mode implements FileInfo.
func (f BasicFileInfo) Mode() fs.FileMode { return f.FileMode }

// ModTime implements FileInfo.
func (f BasicFileInfo) ModTime() time.Time { return f.FileModTime }

// IsDir implements FileInfo.
func (f BasicFileInfo) IsDir() bool { return f.FileIsDir }

// FromOSFileInfo creates a BasicFileInfo from an os.FileInfo.
func FromOSFileInfo(fi fs.FileInfo) BasicFileInfo {
	return BasicFileInfo{
		FileName:    fi.Name(),
		FileSize:    fi.Size(),
		FileMode:    fi.Mode(),
		FileModTime: fi.ModTime(),
		FileIsDir:   fi.IsDir(),
	}
}

// Error types for connection operations.
type (
	// ConnectionError indicates a connection-level failure.
	ConnectionError struct {
		Op      string // Operation that failed (e.g., "connect", "exec")
		Machine string // Machine name or address
		Err     error  // Underlying error
	}

	// NotFoundError indicates a file or resource was not found.
	NotFoundError struct {
		Path string
	}

	// PermissionError indicates an access permission failure.
	PermissionError struct {
		Path string
		Op   string
	}
)

func (e *ConnectionError) Error() string {
	return "connection " + e.Op + " on " + e.Machine + ": " + e.Err.Error()
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

func (e *NotFoundError) Error() string {
	return "not found: " + e.Path
}

func (e *PermissionError) Error() string {
	return "permission denied: " + e.Op + " " + e.Path
}

// Package debug provides timestamped debug logging for gt commands.
// Enable with GT_DEBUG=1 environment variable.
package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	enabled  bool
	logFile  *os.File
	mu       sync.Mutex
	initOnce sync.Once
)

const logPath = "/tmp/gt-logs/gt-debug.log"

// Init initializes debug logging. Called automatically on first Log call.
func Init() {
	initOnce.Do(func() {
		if os.Getenv("GT_DEBUG") != "" {
			enabled = true
			// Ensure log directory exists
			if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
				fmt.Fprintf(os.Stderr, "debug: could not create log dir: %v\n", err)
				return
			}
			// Open log file for append
			f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "debug: could not open log file: %v\n", err)
				return
			}
			logFile = f
		}
	})
}

// Enabled returns true if debug logging is enabled.
func Enabled() bool {
	Init()
	return enabled
}

// Log writes a timestamped debug message to the log file.
// Format: [YYYY-MM-DD HH:MM:SS.mmm] [component] message
func Log(component, format string, args ...interface{}) {
	Init()
	if !enabled || logFile == nil {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(logFile, "[%s] [%s] %s\n", timestamp, component, msg)
	logFile.Sync() // Flush immediately for debugging
}

// LogStart logs the start of an operation with a unique ID for correlation.
func LogStart(component, operation string) string {
	id := fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	Log(component, "START %s (id=%s)", operation, id)
	return id
}

// LogEnd logs the end of an operation with duration.
func LogEnd(component, operation, id string, err error) {
	if err != nil {
		Log(component, "END %s (id=%s) ERROR: %v", operation, id, err)
	} else {
		Log(component, "END %s (id=%s) OK", operation, id)
	}
}

// Close closes the log file. Call on program exit.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

package archive

import (
	"time"
)

// Default configuration values.
const (
	DefaultInterval         = 1 * time.Second
	DefaultHeight           = 100
	DefaultWidth            = 120
	DefaultStoragePath      = "/gt/.logs"
	DefaultScrollThreshold  = 0.1 // 10% overlap required for scroll detection
	DefaultMaxMyersDistance = 50  // Abandon Myers if edit distance exceeds this
	DefaultMyersRateLimit   = 500 * time.Millisecond // Min time between Myers runs
)

// Config holds configuration for the Archiver.
type Config struct {
	// Interval is how often to capture tmux pane output.
	// Default: 1 second
	Interval time.Duration

	// Height is the number of lines to capture from the pane.
	// Default: 100
	Height int

	// Width is the expected terminal width for formatting.
	// Default: 120
	Width int

	// StoragePath is where journal files are stored.
	// Default: /gt/.logs
	StoragePath string

	// ScrollThreshold is the minimum overlap ratio for scroll detection.
	// A value of 0.1 means at least 10% of lines must overlap.
	// Default: 0.1
	ScrollThreshold float64

	// MaxMyersDistance is the maximum edit distance for Myers diff.
	// If exceeded, Myers is abandoned and we do a full redraw.
	// Default: 50
	MaxMyersDistance int

	// MyersRateLimit is the minimum time between Myers diff runs.
	// This prevents expensive diffs from running too frequently.
	// Default: 500ms
	MyersRateLimit time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Interval:         DefaultInterval,
		Height:           DefaultHeight,
		Width:            DefaultWidth,
		StoragePath:      DefaultStoragePath,
		ScrollThreshold:  DefaultScrollThreshold,
		MaxMyersDistance: DefaultMaxMyersDistance,
		MyersRateLimit:   DefaultMyersRateLimit,
	}
}

// Option is a functional option for configuring an Archiver.
type Option func(*Config)

// WithInterval sets the capture interval.
func WithInterval(d time.Duration) Option {
	return func(c *Config) {
		c.Interval = d
	}
}

// WithHeight sets the number of lines to capture.
func WithHeight(h int) Option {
	return func(c *Config) {
		c.Height = h
	}
}

// WithWidth sets the expected terminal width.
func WithWidth(w int) Option {
	return func(c *Config) {
		c.Width = w
	}
}

// WithStoragePath sets the journal storage directory.
func WithStoragePath(path string) Option {
	return func(c *Config) {
		c.StoragePath = path
	}
}

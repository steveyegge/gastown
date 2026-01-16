package archive

import (
	"time"
)

// Default configuration values.
const (
	DefaultInterval    = 1 * time.Second
	DefaultHeight      = 100
	DefaultWidth       = 120
	DefaultStoragePath = "/gt/.logs"
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
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Interval:    DefaultInterval,
		Height:      DefaultHeight,
		Width:       DefaultWidth,
		StoragePath: DefaultStoragePath,
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

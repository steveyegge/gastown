// Package config provides controller configuration from flags and environment.
package config

import (
	"flag"
	"os"
	"strconv"
	"time"
)

// Config holds controller configuration. Values come from flags, env vars,
// or defaults, in that priority order.
type Config struct {
	// DaemonHost is the BD Daemon hostname (env: BD_DAEMON_HOST).
	DaemonHost string

	// DaemonPort is the BD Daemon RPC port (env: BD_DAEMON_PORT).
	DaemonPort int

	// DaemonHTTPPort is the BD Daemon HTTP port for SSE events (env: BD_DAEMON_HTTP_PORT).
	DaemonHTTPPort int

	// DaemonToken is the BD Daemon auth token (env: BD_DAEMON_TOKEN).
	DaemonToken string

	// Namespace is the K8s namespace to operate in (env: NAMESPACE).
	Namespace string

	// KubeConfig is the path to kubeconfig file (env: KUBECONFIG).
	// Empty means use in-cluster config.
	KubeConfig string

	// LogLevel controls log verbosity: debug, info, warn, error (env: LOG_LEVEL).
	LogLevel string

	// DefaultImage is the default container image for agent pods (env: AGENT_IMAGE).
	DefaultImage string

	// CoopImage is the Coop sidecar container image (env: COOP_IMAGE).
	// When set, agent pods get a Coop sidecar for PTY-based management.
	CoopImage string

	// APIKeySecret is the K8s secret name containing ANTHROPIC_API_KEY (env: API_KEY_SECRET).
	APIKeySecret string

	// CredentialsSecret is the K8s secret containing Claude OAuth credentials (env: CLAUDE_CREDENTIALS_SECRET).
	// Mounted as ~/.claude/.credentials.json in agent pods for Max/Corp accounts.
	CredentialsSecret string

	// DaemonTokenSecret is the K8s secret containing the daemon auth token (env: DAEMON_TOKEN_SECRET).
	// Injected as BD_DAEMON_TOKEN in agent pods.
	DaemonTokenSecret string

	// SyncInterval is how often to reconcile pod statuses with beads (env: SYNC_INTERVAL).
	// Default: 60s.
	SyncInterval time.Duration
}

// Parse reads configuration from flags and environment variables.
// Environment variables override defaults; flags override everything.
func Parse() *Config {
	cfg := &Config{
		DaemonHost:     envOr("BD_DAEMON_HOST", "localhost"),
		DaemonPort:     envIntOr("BD_DAEMON_PORT", 9876),
		DaemonHTTPPort: envIntOr("BD_DAEMON_HTTP_PORT", 9080),
		DaemonToken:    os.Getenv("BD_DAEMON_TOKEN"),
		Namespace:      envOr("NAMESPACE", "gastown"),
		KubeConfig:     os.Getenv("KUBECONFIG"),
		LogLevel:       envOr("LOG_LEVEL", "info"),
		DefaultImage:   os.Getenv("AGENT_IMAGE"),
		CoopImage:      os.Getenv("COOP_IMAGE"),
		APIKeySecret:      os.Getenv("API_KEY_SECRET"),
		CredentialsSecret: os.Getenv("CLAUDE_CREDENTIALS_SECRET"),
		DaemonTokenSecret: os.Getenv("DAEMON_TOKEN_SECRET"),
		SyncInterval:      envDurationOr("SYNC_INTERVAL", 60*time.Second),
	}

	flag.StringVar(&cfg.DaemonHost, "daemon-host", cfg.DaemonHost, "BD Daemon hostname")
	flag.IntVar(&cfg.DaemonPort, "daemon-port", cfg.DaemonPort, "BD Daemon RPC port")
	flag.IntVar(&cfg.DaemonHTTPPort, "daemon-http-port", cfg.DaemonHTTPPort, "BD Daemon HTTP port for SSE events")
	flag.StringVar(&cfg.DaemonToken, "daemon-token", cfg.DaemonToken, "BD Daemon auth token")
	flag.StringVar(&cfg.Namespace, "namespace", cfg.Namespace, "Kubernetes namespace")
	flag.StringVar(&cfg.KubeConfig, "kubeconfig", cfg.KubeConfig, "Path to kubeconfig (empty for in-cluster)")
	flag.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "Log level: debug, info, warn, error")
	flag.StringVar(&cfg.DefaultImage, "agent-image", cfg.DefaultImage, "Default container image for agent pods")
	flag.StringVar(&cfg.CoopImage, "coop-image", cfg.CoopImage, "Coop sidecar container image")
	flag.StringVar(&cfg.APIKeySecret, "api-key-secret", cfg.APIKeySecret, "K8s secret name containing ANTHROPIC_API_KEY")
	flag.StringVar(&cfg.CredentialsSecret, "credentials-secret", cfg.CredentialsSecret, "K8s secret with Claude OAuth credentials")
	flag.StringVar(&cfg.DaemonTokenSecret, "daemon-token-secret", cfg.DaemonTokenSecret, "K8s secret with daemon auth token for agent pods")
	flag.DurationVar(&cfg.SyncInterval, "sync-interval", cfg.SyncInterval, "Interval for periodic pod status sync")
	flag.Parse()

	return cfg
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

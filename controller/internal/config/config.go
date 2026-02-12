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
	// Mutually exclusive with CoopBuiltin — do not set both.
	CoopImage string

	// CoopBuiltin indicates the agent image has coop built into its entrypoint
	// (env: COOP_BUILTIN). When true, the agent container exposes coop HTTP
	// ports and uses HTTP probes instead of exec probes. No sidecar is added.
	CoopBuiltin bool

	// APIKeySecret is the K8s secret name containing ANTHROPIC_API_KEY (env: API_KEY_SECRET).
	APIKeySecret string

	// CredentialsSecret is the K8s secret containing Claude OAuth credentials (env: CLAUDE_CREDENTIALS_SECRET).
	// Mounted as ~/.claude/.credentials.json in agent pods for Max/Corp accounts.
	CredentialsSecret string

	// DaemonTokenSecret is the K8s secret containing the daemon auth token (env: DAEMON_TOKEN_SECRET).
	// Injected as BD_DAEMON_TOKEN in agent pods.
	DaemonTokenSecret string

	// TownName is the Gas Town deployment name (env: GT_TOWN_NAME).
	// Used to set GT_TOWN_NAME in agent pods for workspace + materialization scope.
	TownName string

	// NatsURL is the NATS server URL for event bus (env: NATS_URL).
	// Passed to agent pods as COOP_NATS_URL for real-time events.
	NatsURL string

	// GitCredentialsSecret is the K8s secret containing git credentials (env: GIT_CREDENTIALS_SECRET).
	// Keys "username" and "token" are injected as GIT_USERNAME and GIT_TOKEN env vars
	// in agent pods for git clone/push to GitHub.
	GitCredentialsSecret string

	// NatsTokenSecret is the K8s secret containing the NATS auth token (env: NATS_TOKEN_SECRET).
	// Injected as COOP_NATS_TOKEN in agent pods.
	NatsTokenSecret string

	// CoopBrokerURL is the URL of the central coop broker (env: COOP_BROKER_URL).
	// When set, agent pods register with the broker for credential distribution and mux.
	CoopBrokerURL string

	// CoopBrokerTokenSecret is the K8s secret containing the broker auth token (env: COOP_BROKER_TOKEN_SECRET).
	// Injected as COOP_BROKER_TOKEN in agent pods.
	CoopBrokerTokenSecret string

	// CoopMuxURL is the URL of the coop multiplexer (env: COOP_MUX_URL).
	// When set, agent pods auto-register with the mux for aggregated monitoring.
	CoopMuxURL string

	// Transport selects the event transport: "sse" or "nats" (env: WATCHER_TRANSPORT).
	// Default: "sse".
	Transport string

	// NatsConsumerName is the durable consumer name for JetStream (env: NATS_CONSUMER_NAME).
	// Default: "controller-<namespace>".
	NatsConsumerName string

	// SyncInterval is how often to reconcile pod statuses with beads (env: SYNC_INTERVAL).
	// Default: 60s.
	SyncInterval time.Duration

	// RigCache maps rig name → git mirror service name, populated at runtime
	// from rig beads in the daemon. Not parsed from env/flags.
	RigCache map[string]RigCacheEntry
}

// RigCacheEntry holds rig metadata from daemon rig beads.
type RigCacheEntry struct {
	Prefix        string // e.g., "bd", "gt"
	GitMirrorSvc  string // e.g., "git-mirror-beads"
	GitURL        string // e.g., "https://github.com/groblegark/beads.git"
	DefaultBranch string // e.g., "main"

	// Per-rig pod customization (from rig bead labels).
	Image        string // Override agent image for this rig
	StorageClass string // Override PVC storage class
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
		CoopBuiltin:    envBoolOr("COOP_BUILTIN", false),
		APIKeySecret:      os.Getenv("API_KEY_SECRET"),
		CredentialsSecret: os.Getenv("CLAUDE_CREDENTIALS_SECRET"),
		DaemonTokenSecret: os.Getenv("DAEMON_TOKEN_SECRET"),
		TownName:          envOr("GT_TOWN_NAME", "town"),
		GitCredentialsSecret: os.Getenv("GIT_CREDENTIALS_SECRET"),
		NatsURL:               os.Getenv("NATS_URL"),
		NatsTokenSecret:       os.Getenv("NATS_TOKEN_SECRET"),
		CoopBrokerURL:         os.Getenv("COOP_BROKER_URL"),
		CoopBrokerTokenSecret: os.Getenv("COOP_BROKER_TOKEN_SECRET"),
		CoopMuxURL:            os.Getenv("COOP_MUX_URL"),
		Transport:         envOr("WATCHER_TRANSPORT", "sse"),
		NatsConsumerName:  os.Getenv("NATS_CONSUMER_NAME"),
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
	flag.BoolVar(&cfg.CoopBuiltin, "coop-builtin", cfg.CoopBuiltin, "Agent image has coop built-in (HTTP probes, no sidecar)")
	flag.StringVar(&cfg.APIKeySecret, "api-key-secret", cfg.APIKeySecret, "K8s secret name containing ANTHROPIC_API_KEY")
	flag.StringVar(&cfg.CredentialsSecret, "credentials-secret", cfg.CredentialsSecret, "K8s secret with Claude OAuth credentials")
	flag.StringVar(&cfg.DaemonTokenSecret, "daemon-token-secret", cfg.DaemonTokenSecret, "K8s secret with daemon auth token for agent pods")
	flag.StringVar(&cfg.TownName, "town-name", cfg.TownName, "Gas Town deployment name")
	flag.StringVar(&cfg.GitCredentialsSecret, "git-credentials-secret", cfg.GitCredentialsSecret, "K8s secret with git credentials (username/token keys)")
	flag.StringVar(&cfg.NatsURL, "nats-url", cfg.NatsURL, "NATS server URL for event bus")
	flag.StringVar(&cfg.NatsTokenSecret, "nats-token-secret", cfg.NatsTokenSecret, "K8s secret with NATS auth token")
	flag.StringVar(&cfg.CoopBrokerURL, "coop-broker-url", cfg.CoopBrokerURL, "URL of the central coop broker")
	flag.StringVar(&cfg.CoopBrokerTokenSecret, "coop-broker-token-secret", cfg.CoopBrokerTokenSecret, "K8s secret with broker auth token")
	flag.StringVar(&cfg.CoopMuxURL, "coop-mux-url", cfg.CoopMuxURL, "URL of the coop multiplexer for session aggregation")
	flag.StringVar(&cfg.Transport, "transport", cfg.Transport, "Event transport: sse or nats")
	flag.StringVar(&cfg.NatsConsumerName, "nats-consumer-name", cfg.NatsConsumerName, "Durable consumer name for JetStream")
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

func envBoolOr(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			return b
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

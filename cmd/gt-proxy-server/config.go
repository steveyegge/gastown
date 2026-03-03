package main

import (
	"encoding/json"
	"errors"
	"os"
)

// ProxyConfig is the configuration file schema for gt-proxy-server.
// It is loaded from JSON and merged with CLI flags (flags take precedence).
// The default location is ~/gt/.runtime/proxy/config.json.
type ProxyConfig struct {
	// ListenAddr is the address and port to listen on (e.g. "0.0.0.0:9876").
	ListenAddr string `json:"listen_addr"`

	// AdminListenAddr is the address for the local admin HTTP server.
	// Defaults to "127.0.0.1:9877". Set to "" to disable.
	AdminListenAddr string `json:"admin_listen_addr"`

	// CADir is the directory holding ca.crt and ca.key.
	// Defaults to ~/gt/.runtime/ca if empty.
	CADir string `json:"ca_dir"`

	// TownRoot is the Gas Town root directory (e.g. ~/gt).
	// Defaults to $GT_TOWN or ~/gt if empty.
	TownRoot string `json:"town_root"`

	// AllowedCommands is the list of binary names polecats may execute (e.g. ["gt","bd"]).
	AllowedCommands []string `json:"allowed_commands"`

	// AllowedSubcommands maps each allowed command to the subcommands polecats
	// may invoke. Subcommands not listed here are rejected with 403.
	AllowedSubcommands map[string][]string `json:"allowed_subcommands"`

	// ExtraSANIPs lists additional IP addresses to embed as IP Subject Alternative
	// Names in the server TLS certificate.
	//
	// Use this for addresses that containers use to reach the proxy but that are
	// not local interface addresses (and therefore not auto-detected):
	//   - External/NAT IP:  the router's public IP, if you have port forwarding
	//     configured so that containers can reach the proxy from the internet.
	//   - VPN tunnel IP:    the IP assigned to the VPN interface on the remote side.
	//   - Additional LAN IPs: if the host has multiple NICs or aliases.
	//
	// Note: the NAT exit IP (e.g. the IP shown by "curl ifconfig.me") cannot be
	// auto-detected because it is assigned to the router, not to any interface on
	// this machine. It must be listed here explicitly if containers need to connect
	// through NAT.
	ExtraSANIPs []string `json:"extra_san_ips"`

	// ExtraSANHosts lists additional DNS names to embed as DNS Subject Alternative
	// Names in the server TLS certificate.
	//
	// Use this for hostnames that containers resolve to reach the proxy, such as:
	//   - A hostname in the container's /etc/hosts
	//   - A split-horizon DNS entry that resolves to the proxy IP
	//   - A mDNS name (e.g. "macbook.local")
	ExtraSANHosts []string `json:"extra_san_hosts"`
}

// loadConfig reads the config file at path and returns a ProxyConfig.
// If the file does not exist, an empty ProxyConfig is returned (not an error).
// JSON parse errors are returned as errors.
func loadConfig(path string) (ProxyConfig, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from internal config location
	if errors.Is(err, os.ErrNotExist) {
		return ProxyConfig{}, nil
	}
	if err != nil {
		return ProxyConfig{}, err
	}
	var cfg ProxyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ProxyConfig{}, err
	}
	return cfg, nil
}

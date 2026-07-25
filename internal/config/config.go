package config

import (
	"os"
	"strings"
)

// defaultAgentPorts are the candidate ports the worker agent binds to, tried in
// order. They sit above the well-known services (9090 is Prometheus and Cockpit,
// 9100 node_exporter, 4646-4648 Nomad) and below Linux's default ephemeral range
// of 32768-60999, so they cannot collide with an outgoing connection's source port.
const defaultAgentPorts = "19090,19091,19092"

type Config struct {
	DatabaseURL   string
	Port          string
	EncryptionKey string
	DashboardURL  string
	AgentPorts    []string
}

func Load() *Config {
	return &Config{
		DatabaseURL:   env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/orchestrator?sslmode=disable"),
		Port:          env("PORT", "8080"),
		EncryptionKey: env("ENCRYPTION_KEY", "orchestrator-secret-key-32bytes!"),
		DashboardURL:  env("DASHBOARD_URL", "http://localhost:8080"),
		AgentPorts:    ParsePorts(env("AGENT_PORTS", defaultAgentPorts)),
	}
}

// ParsePorts splits a comma-separated port list, ignoring blanks and whitespace.
// Falls back to the defaults if nothing usable remains.
func ParsePorts(list string) []string {
	var ports []string
	for _, p := range strings.Split(list, ",") {
		if p = strings.TrimSpace(p); p != "" {
			ports = append(ports, p)
		}
	}
	if len(ports) == 0 {
		return strings.Split(defaultAgentPorts, ",")
	}
	return ports
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

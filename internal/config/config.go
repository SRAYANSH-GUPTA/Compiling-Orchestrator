package config

import "os"

type Config struct {
	DatabaseURL   string
	Port          string
	EncryptionKey string
	DashboardURL  string
	AgentPort     string
}

func Load() *Config {
	return &Config{
		DatabaseURL:   env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/orchestrator?sslmode=disable"),
		Port:          env("PORT", "8080"),
		EncryptionKey: env("ENCRYPTION_KEY", "orchestrator-secret-key-32bytes!"),
		DashboardURL:  env("DASHBOARD_URL", "http://localhost:8080"),
		AgentPort:     env("AGENT_PORT", "9090"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

package main

import (
	"log"
	"os"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/agent"
)

func main() {
	workerUUID := mustEnv("WORKER_UUID")
	apiKey := mustEnv("API_KEY")
	dashboardURL := mustEnv("DASHBOARD_URL")
	port := envOr("AGENT_PORT", "9090")

	sender := agent.NewHeartbeatSender(dashboardURL, apiKey, workerUUID)
	go sender.Start()

	server := agent.NewServer(apiKey, workerUUID, port)
	if err := server.Run(); err != nil {
		log.Fatalf("agent server: %v", err)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

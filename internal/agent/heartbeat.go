package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type HeartbeatSender struct {
	dashboardURL string
	apiKey       string
	workerUUID   string
	startTime    time.Time
	totalJobs    int64
}

func NewHeartbeatSender(dashboardURL, apiKey, workerUUID string) *HeartbeatSender {
	return &HeartbeatSender{
		dashboardURL: dashboardURL,
		apiKey:       apiKey,
		workerUUID:   workerUUID,
		startTime:    time.Now(),
	}
}

func (s *HeartbeatSender) Start() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := s.send(); err != nil {
			log.Printf("heartbeat: %v", err)
		}
	}
}

func (s *HeartbeatSender) send() error {
	m := CollectMetrics()

	payload := map[string]interface{}{
		"worker_uuid":    s.workerUUID,
		"cpu_usage":      m.CPUUsage,
		"ram_usage":      m.RAMUsage,
		"disk_usage":     m.DiskUsage,
		"network_rx":     m.NetworkRX,
		"network_tx":     m.NetworkTX,
		"docker_status":  m.DockerStatus,
		"nomad_status":   m.NomadStatus,
		"judge_status":   m.JudgeStatus,
		"running_jobs":   m.RunningJobs,
		"queue_length":   0,
		"total_requests": s.totalJobs,
		"avg_runtime":    0.0,
		"retry_count":    0,
		"worker_version": "1.0.0",
		"uptime":         int64(time.Since(s.startTime).Seconds()),
		"hostname":       m.Hostname,
		"nomad_node_id":  "",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", s.dashboardURL+"/api/heartbeat", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", s.apiKey)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dashboard returned %d", resp.StatusCode)
	}
	return nil
}

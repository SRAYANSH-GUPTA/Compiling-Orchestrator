package models

import (
	"fmt"
	"time"
)

type ProvisionStatus string

const (
	StatusPending      ProvisionStatus = "pending"
	StatusProvisioning ProvisionStatus = "provisioning"
	StatusProvisioned  ProvisionStatus = "provisioned"
	StatusFailed       ProvisionStatus = "failed"
	StatusOnline       ProvisionStatus = "online"
	StatusOffline      ProvisionStatus = "offline"
	StatusDraining     ProvisionStatus = "draining"
	StatusDisabled     ProvisionStatus = "disabled"
)

type Worker struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Hostname             string          `json:"hostname"`
	IPAddress            string          `json:"ip_address"`
	SSHUsername          string          `json:"ssh_username"`
	SSHPasswordEncrypted string          `json:"-"`
	SSHPublicKey         string          `json:"ssh_public_key,omitempty"`
	APIKey               string          `json:"-"`
	ProvisionStatus      ProvisionStatus `json:"provision_status"`
	LastHeartbeat        *time.Time      `json:"last_heartbeat,omitempty"`
	WorkerUUID           string          `json:"worker_uuid,omitempty"`
	NomadNodeID          string          `json:"nomad_node_id,omitempty"`
	CPUUsage             float64         `json:"cpu_usage"`
	RAMUsage             float64         `json:"ram_usage"`
	DiskUsage            float64         `json:"disk_usage"`
	NetworkRX            int64           `json:"network_rx"`
	NetworkTX            int64           `json:"network_tx"`
	DockerStatus         string          `json:"docker_status"`
	NomadStatus          string          `json:"nomad_status"`
	JudgeStatus          string          `json:"judge_status"`
	RunningJobs          int             `json:"running_jobs"`
	TotalJobs            int64           `json:"total_jobs"`
	FailedJobs           int64           `json:"failed_jobs"`
	AvgRuntime           float64         `json:"avg_runtime"`
	Uptime               int64           `json:"uptime"`
	WorkerVersion        string          `json:"worker_version"`
	CPUModel             string          `json:"cpu_model"`
	CPUCores             int             `json:"cpu_cores"`
	TotalRAM             int64           `json:"total_ram"`
	TotalDisk            int64           `json:"total_disk"`
	OSVersion            string          `json:"os_version"`
	KernelVersion        string          `json:"kernel_version"`
	DockerVersion        string          `json:"docker_version"`
	NomadVersion         string          `json:"nomad_version"`
	JudgeVersion         string          `json:"judge_version"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (w *Worker) IsOnline() bool {
	if w.LastHeartbeat == nil {
		return false
	}
	return time.Since(*w.LastHeartbeat) < 15*time.Second
}

func (w *Worker) HealthColor() string {
	if !w.IsOnline() || w.ProvisionStatus == StatusOffline || w.ProvisionStatus == StatusDisabled {
		return "red"
	}
	if w.CPUUsage > 85 || w.RAMUsage > 85 {
		return "yellow"
	}
	return "green"
}

func (w *Worker) SuccessRate() float64 {
	if w.TotalJobs == 0 {
		return 100.0
	}
	return float64(w.TotalJobs-w.FailedJobs) / float64(w.TotalJobs) * 100
}

func (w *Worker) UptimeStr() string {
	secs := w.Uptime
	days := secs / 86400
	hours := (secs % 86400) / 3600
	mins := (secs % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}

type ProvisionLog struct {
	ID        int64     `json:"id"`
	WorkerID  string    `json:"worker_id"`
	Step      string    `json:"step"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type Heartbeat struct {
	WorkerUUID    string  `json:"worker_uuid"`
	CPUUsage      float64 `json:"cpu_usage"`
	RAMUsage      float64 `json:"ram_usage"`
	DiskUsage     float64 `json:"disk_usage"`
	NetworkRX     int64   `json:"network_rx"`
	NetworkTX     int64   `json:"network_tx"`
	DockerStatus  string  `json:"docker_status"`
	NomadStatus   string  `json:"nomad_status"`
	JudgeStatus   string  `json:"judge_status"`
	RunningJobs   int     `json:"running_jobs"`
	QueueLength   int     `json:"queue_length"`
	TotalRequests int64   `json:"total_requests"`
	AvgRuntime    float64 `json:"avg_runtime"`
	RetryCount    int     `json:"retry_count"`
	WorkerVersion string  `json:"worker_version"`
	Uptime        int64   `json:"uptime"`
	Hostname      string  `json:"hostname"`
	NomadNodeID   string  `json:"nomad_node_id"`
}

type Notification struct {
	ID        int64     `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	WorkerID  string    `json:"worker_id,omitempty"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

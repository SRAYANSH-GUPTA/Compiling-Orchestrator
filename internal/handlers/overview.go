package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type overviewData struct {
	Page                string
	WorkerCount         int
	TotalWorkers        int
	OnlineWorkers       int
	OfflineWorkers      int
	DrainingWorkers     int
	ProvisioningWorkers int
	ActiveJobs          int
	AvgCPU              float64
	AvgRAM              float64
	Services            []serviceStatus
	Notifications       []*models.Notification
}

type serviceStatus struct {
	Name   string
	Status string
}

type clusterStats struct {
	TotalWorkers        int
	OnlineWorkers       int
	OfflineWorkers      int
	DrainingWorkers     int
	ProvisioningWorkers int
	ActiveJobs          int
	AvgCPU              float64
	AvgRAM              float64
}

func Overview(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workers, _ := repo.List(r.Context())
		data := buildOverviewData(workers)
		data.Page = "overview"
		ns, _ := repo.GetUnreadNotifications(r.Context())
		data.Notifications = ns
		render(w, "overview.html", data)
	}
}

func StatsAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workers, _ := repo.List(r.Context())
		data := buildOverviewData(workers)
		render(w, "stats-grid", data)
	}
}

func ClusterHealthAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workers, _ := repo.List(r.Context())
		online, total := 0, len(workers)
		for _, wk := range workers {
			if wk.IsOnline() {
				online++
			}
		}
		status := "healthy"
		dotClass := "dot-green"
		if total == 0 || online == 0 {
			status = "no workers"
			dotClass = "dot-gray"
		} else if online < total {
			status = "degraded"
			dotClass = "dot-yellow"
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<div id="cluster-health-badge"><div class="cluster-health">` +
			`<span class="dot ` + dotClass + `"></span> ` + status + `</div></div>`))
	}
}

func ServiceStatusAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workers, _ := repo.List(r.Context())
		data := buildOverviewData(workers)
		render(w, "service-status", data)
	}
}

func NotificationsAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns, _ := repo.GetUnreadNotifications(r.Context())
		render(w, "notifications", map[string]interface{}{"Notifications": ns})
	}
}

func NotificationsJSON(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ns, _ := repo.GetUnreadNotifications(r.Context())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ns)
	}
}

func buildOverviewData(workers []*models.Worker) *overviewData {
	data := &overviewData{}
	data.TotalWorkers = len(workers)
	data.WorkerCount = len(workers)

	var totalCPU, totalRAM float64
	onlineCount := 0

	for _, w := range workers {
		switch w.ProvisionStatus {
		case models.StatusOnline:
			data.OnlineWorkers++
		case models.StatusOffline:
			data.OfflineWorkers++
		case models.StatusDraining:
			data.DrainingWorkers++
		case models.StatusProvisioning:
			data.ProvisioningWorkers++
		}
		data.ActiveJobs += w.RunningJobs
		if w.IsOnline() {
			totalCPU += w.CPUUsage
			totalRAM += w.RAMUsage
			onlineCount++
		}
	}

	if onlineCount > 0 {
		data.AvgCPU = totalCPU / float64(onlineCount)
		data.AvgRAM = totalRAM / float64(onlineCount)
	}

	data.Services = buildServiceStatus(workers)
	return data
}

func buildServiceStatus(workers []*models.Worker) []serviceStatus {
	if len(workers) == 0 {
		return []serviceStatus{
			{Name: "Docker", Status: "no workers"},
			{Name: "Nomad", Status: "no workers"},
			{Name: "Judge0", Status: "no workers"},
		}
	}
	docker, nomad, judge := "up", "up", "up"
	for _, w := range workers {
		if w.IsOnline() {
			if w.DockerStatus != "running" {
				docker = "degraded"
			}
			if w.NomadStatus != "running" {
				nomad = "degraded"
			}
			if w.JudgeStatus != "running" {
				judge = "degraded"
			}
		}
	}
	return []serviceStatus{
		{Name: "Docker", Status: docker},
		{Name: "Nomad", Status: nomad},
		{Name: "Judge0", Status: judge},
	}
}

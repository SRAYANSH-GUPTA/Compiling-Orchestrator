package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/config"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type ActionHandler struct {
	repo *repository.WorkerRepo
	cfg  *config.Config
}

func NewActionHandler(repo *repository.WorkerRepo, cfg *config.Config) *ActionHandler {
	return &ActionHandler{repo: repo, cfg: cfg}
}

// agentDo sends req to each candidate agent port until one answers. A worker's
// agent binds the first free port from the same list, so which one it landed on
// is not known ahead of time. Only connection-level failures fall through to the
// next port; an HTTP response of any status means we found the agent.
func (h *ActionHandler) agentDo(ip string, build func(url string) (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for _, port := range h.cfg.AgentPorts {
		req, err := build(fmt.Sprintf("http://%s:%s", ip, port))
		if err != nil {
			return nil, err
		}
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("agent unreachable on ports %v: %w", h.cfg.AgentPorts, lastErr)
}

func (h *ActionHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "worker not found", http.StatusNotFound)
		return
	}
	if err := h.agentPost(worker, "/health", nil); err != nil {
		h.repo.AddProvisionLog(r.Context(), &models.ProvisionLog{
			WorkerID: id, Step: "test-connection", Status: "failed",
			Message: "Agent unreachable: " + err.Error(),
		})
	} else {
		h.repo.AddProvisionLog(r.Context(), &models.ProvisionLog{
			WorkerID: id, Step: "test-connection", Status: "ok",
			Message: "Connection to agent successful",
		})
	}
	h.repo.AddAuditLog(r.Context(), "test_connection", id, "")
	http.Redirect(w, r, "/workers/"+id, http.StatusFound)
}

func (h *ActionHandler) RestartDocker(w http.ResponseWriter, r *http.Request) {
	h.agentAction(w, r, "/restart", map[string]string{"service": "docker"}, "restart_docker")
}

func (h *ActionHandler) RestartNomad(w http.ResponseWriter, r *http.Request) {
	h.agentAction(w, r, "/restart", map[string]string{"service": "nomad"}, "restart_nomad")
}

func (h *ActionHandler) RestartAgent(w http.ResponseWriter, r *http.Request) {
	h.agentAction(w, r, "/restart", map[string]string{"service": "worker-agent"}, "restart_agent")
}

func (h *ActionHandler) RestartJudge(w http.ResponseWriter, r *http.Request) {
	h.agentAction(w, r, "/restart", map[string]string{"service": "judge"}, "restart_judge")
}

func (h *ActionHandler) Drain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.repo.UpdateStatus(r.Context(), id, models.StatusDraining)
	h.agentAction(w, r, "/drain", nil, "drain_worker")
}

func (h *ActionHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.repo.UpdateStatus(r.Context(), id, models.StatusOnline)
	h.agentAction(w, r, "/resume", nil, "resume_worker")
}

func (h *ActionHandler) Update(w http.ResponseWriter, r *http.Request) {
	h.agentAction(w, r, "/update", nil, "update_worker")
}

func (h *ActionHandler) CleanDocker(w http.ResponseWriter, r *http.Request) {
	h.agentAction(w, r, "/cleanup", nil, "clean_docker")
}

func (h *ActionHandler) RemoveWorker(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.agentPost(&models.Worker{}, "/shutdown", nil)
	h.repo.Delete(r.Context(), id)
	h.repo.AddAuditLog(r.Context(), "worker_removed", id, "")
	http.Redirect(w, r, "/workers", http.StatusFound)
}

func (h *ActionHandler) agentAction(w http.ResponseWriter, r *http.Request, path string, payload interface{}, auditAction string) {
	id := chi.URLParam(r, "id")
	worker, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.agentPost(worker, path, payload); err != nil {
		h.repo.AddProvisionLog(r.Context(), &models.ProvisionLog{
			WorkerID: id, Step: auditAction, Status: "failed",
			Message: fmt.Sprintf("Agent command %s failed: %v", path, err),
		})
	}
	h.repo.AddAuditLog(r.Context(), auditAction, id, "")
	http.Redirect(w, r, "/workers/"+id, http.StatusFound)
}

func (h *ActionHandler) agentPost(worker *models.Worker, path string, payload interface{}) error {
	if worker.IPAddress == "" {
		return fmt.Errorf("worker has no IP address")
	}
	var body bytes.Buffer
	if payload != nil {
		json.NewEncoder(&body).Encode(payload)
	}

	resp, err := h.agentDo(worker.IPAddress, func(base string) (*http.Request, error) {
		req, err := http.NewRequest("POST", base+path, bytes.NewReader(body.Bytes()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-API-Key", worker.APIKey)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("agent returned %d", resp.StatusCode)
	}
	return nil
}

func (h *ActionHandler) UpdateAll(w http.ResponseWriter, r *http.Request) {
	workers, err := h.repo.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var onlineWorkers []*models.Worker
	for _, wk := range workers {
		if wk.ProvisionStatus == models.StatusOnline {
			onlineWorkers = append(onlineWorkers, wk)
		}
	}

	if len(onlineWorkers) == 0 {
		http.Redirect(w, r, "/workers", http.StatusFound)
		return
	}

	go h.runRollingUpdate(onlineWorkers)

	h.repo.AddAuditLog(r.Context(), "update_all_initiated", "", fmt.Sprintf("Started rolling update for %d workers", len(onlineWorkers)))
	http.Redirect(w, r, "/workers", http.StatusFound)
}

func (h *ActionHandler) runRollingUpdate(workers []*models.Worker) {
	ctx := context.Background()
	for _, wk := range workers {
		h.repo.AddNotification(ctx, &models.Notification{
			Type:     "info",
			Title:    "Updating Worker",
			Message:  fmt.Sprintf("Starting rolling update for %s (%s)", wk.Name, wk.IPAddress),
			WorkerID: wk.ID,
		})

		if err := h.agentPost(wk, "/update", nil); err != nil {
			h.repo.AddNotification(ctx, &models.Notification{
				Type:     "error",
				Title:    "Update Failed",
				Message:  fmt.Sprintf("Failed to trigger update on %s: %v. Rolling update aborted.", wk.Name, err),
				WorkerID: wk.ID,
			})
			return
		}

		healthy := false
		timeout := time.After(3 * time.Minute)
		ticker := time.NewTicker(5 * time.Second)

		var m struct {
			DockerStatus string `json:"docker_status"`
			NomadStatus  string `json:"nomad_status"`
			JudgeStatus  string `json:"judge_status"`
		}

	pollLoop:
		for {
			select {
			case <-timeout:
				ticker.Stop()
				h.repo.AddNotification(ctx, &models.Notification{
					Type:     "error",
					Title:    "Update Timeout",
					Message:  fmt.Sprintf("Worker %s failed to become healthy within timeout. Rolling update aborted.", wk.Name),
					WorkerID: wk.ID,
				})
				return
			case <-ticker.C:
				resp, err := h.agentDo(wk.IPAddress, func(base string) (*http.Request, error) {
					req, err := http.NewRequest("GET", base+"/health", nil)
					if err != nil {
						return nil, err
					}
					req.Header.Set("X-API-Key", wk.APIKey)
					return req, nil
				})
				if err != nil {
					continue
				}

				if resp.StatusCode == 200 {
					json.NewDecoder(resp.Body).Decode(&m)
					resp.Body.Close()
					if m.DockerStatus == "running" && m.NomadStatus == "running" && m.JudgeStatus == "running" {
						healthy = true
						break pollLoop
					}
				} else {
					resp.Body.Close()
				}
			}
		}
		ticker.Stop()

		if healthy {
			h.repo.AddNotification(ctx, &models.Notification{
				Type:     "success",
				Title:    "Worker Updated",
				Message:  fmt.Sprintf("Worker %s successfully updated and is healthy.", wk.Name),
				WorkerID: wk.ID,
			})
		}
	}
}

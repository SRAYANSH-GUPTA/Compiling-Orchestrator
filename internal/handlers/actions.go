package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type ActionHandler struct {
	repo *repository.WorkerRepo
}

func NewActionHandler(repo *repository.WorkerRepo) *ActionHandler {
	return &ActionHandler{repo: repo}
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
	url := fmt.Sprintf("http://%s:9090%s", worker.IPAddress, path)

	var body bytes.Buffer
	if payload != nil {
		json.NewEncoder(&body).Encode(payload)
	}

	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", worker.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("agent returned %d", resp.StatusCode)
	}
	return nil
}

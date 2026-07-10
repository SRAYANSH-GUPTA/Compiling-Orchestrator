package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/config"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/crypto"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/provision"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type ProvisionHandler struct {
	repo    *repository.WorkerRepo
	cfg     *config.Config
	ansible *provision.AnsibleRunner
}

func NewProvisionHandler(repo *repository.WorkerRepo, cfg *config.Config) *ProvisionHandler {
	return &ProvisionHandler{
		repo:    repo,
		cfg:     cfg,
		ansible: provision.NewAnsibleRunner(filepath.Join("ansible", "playbooks")),
	}
}

func (h *ProvisionHandler) AddWorker(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	ip := r.FormValue("ip_address")
	sshUser := r.FormValue("ssh_username")
	sshPass := r.FormValue("ssh_password")

	if name == "" || ip == "" || sshUser == "" || sshPass == "" {
		render(w, "add_worker.html", map[string]interface{}{
			"Error": "All fields are required",
			"Page":  "add-worker",
		})
		return
	}

	encPass, err := crypto.Encrypt(h.cfg.EncryptionKey, sshPass)
	if err != nil {
		render(w, "add_worker.html", map[string]interface{}{
			"Error": "Encryption failed: " + err.Error(),
			"Page":  "add-worker",
		})
		return
	}

	workerUUID := uuid.New().String()
	apiKey := generateAPIKey()

	worker := &models.Worker{
		Name:                 name,
		IPAddress:            ip,
		SSHUsername:          sshUser,
		SSHPasswordEncrypted: encPass,
		WorkerUUID:           workerUUID,
		APIKey:               apiKey,
		ProvisionStatus:      models.StatusPending,
	}

	if err := h.repo.Create(r.Context(), worker); err != nil {
		render(w, "add_worker.html", map[string]interface{}{
			"Error": "Failed to save worker: " + err.Error(),
			"Page":  "add-worker",
		})
		return
	}

	h.repo.AddAuditLog(r.Context(), "worker_added", worker.ID, "Added worker "+name+" at "+ip)
	h.repo.AddNotification(r.Context(), &models.Notification{
		Type:     "info",
		Title:    "Worker Added",
		Message:  name + " (" + ip + ") was added and is pending provisioning.",
		WorkerID: worker.ID,
	})

	go h.runProvision(worker.ID, worker.IPAddress, worker.SSHUsername, sshPass, workerUUID, apiKey)

	http.Redirect(w, r, "/workers/"+worker.ID, http.StatusFound)
}

func (h *ProvisionHandler) ProvisionWorker(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	sshPass, err := crypto.Decrypt(h.cfg.EncryptionKey, worker.SSHPasswordEncrypted)
	if err != nil {
		http.Error(w, "cannot decrypt SSH password", http.StatusInternalServerError)
		return
	}

	go h.runProvision(worker.ID, worker.IPAddress, worker.SSHUsername, sshPass, worker.WorkerUUID, worker.APIKey)
	http.Redirect(w, r, "/workers/"+id, http.StatusFound)
}

func (h *ProvisionHandler) runProvision(workerID, ip, sshUser, sshPass, workerUUID, apiKey string) {
	ctx := context.Background()

	logStep := func(step, status, msg string) {
		h.repo.AddProvisionLog(ctx, &models.ProvisionLog{
			WorkerID: workerID,
			Step:     step,
			Status:   status,
			Message:  msg,
		})
	}

	h.repo.UpdateStatus(ctx, workerID, models.StatusProvisioning)
	logStep("init", "info", "Starting provisioning for "+ip)

	logStep("ssh-test", "running", "Testing SSH connection...")
	if err := provision.TestConnection(ip, sshUser, sshPass); err != nil {
		logStep("ssh-test", "failed", "SSH connection failed: "+err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		h.repo.AddNotification(ctx, &models.Notification{
			Type:     "error",
			Title:    "Provision Failed",
			Message:  "SSH connection to " + ip + " failed: " + err.Error(),
			WorkerID: workerID,
		})
		return
	}
	logStep("ssh-test", "ok", "SSH connection successful")

	logStep("os-check", "running", "Verifying Ubuntu...")
	if err := provision.CheckUbuntu(ip, sshUser, sshPass); err != nil {
		logStep("os-check", "failed", err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		return
	}
	logStep("os-check", "ok", "Ubuntu verified")

	logStep("ansible", "running", "Running Ansible provisioning playbook...")
	err := h.ansible.Provision(ctx, ip, sshUser, sshPass, workerUUID, apiKey, h.cfg.DashboardURL, h.cfg.AgentPort,
		func(step, status, msg string) {
			logStep(step, status, msg)
		},
	)
	if err != nil {
		logStep("ansible", "failed", "Ansible failed: "+err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		h.repo.AddNotification(ctx, &models.Notification{
			Type:     "error",
			Title:    "Provision Failed",
			Message:  "Ansible provisioning failed for " + ip + ": " + err.Error(),
			WorkerID: workerID,
		})
		log.Printf("provision %s failed: %v", ip, err)
		return
	}

	logStep("ssh-key", "running", "Installing SSH public key on worker...")
	privKey, pubKey, err := provision.GetOrCreateSystemSSHKey(filepath.Join("config", "id_rsa"))
	if err != nil {
		logStep("ssh-key", "failed", "SSH key generation failed: "+err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		return
	}
	if err := provision.InstallSSHKey(ip, sshUser, sshPass, pubKey); err != nil {
		logStep("ssh-key", "failed", "Failed to install SSH key: "+err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		return
	}
	logStep("ssh-key", "ok", "SSH public key installed successfully")

	logStep("ssh-disable-password", "running", "Disabling SSH password authentication on worker...")
	if err := provision.DisablePasswordAuth(ip, sshUser, privKey); err != nil {
		logStep("ssh-disable-password", "failed", "Failed to disable password auth: "+err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		return
	}
	logStep("ssh-disable-password", "ok", "SSH password authentication disabled on worker")

	logStep("db-cleanup", "running", "Removing encrypted password from database...")
	if err := h.repo.UpdateSSHKey(ctx, workerID, pubKey); err != nil {
		logStep("db-cleanup", "failed", "Failed to remove encrypted password: "+err.Error())
		h.repo.UpdateStatus(ctx, workerID, models.StatusFailed)
		return
	}
	logStep("db-cleanup", "ok", "Database cleaned up")

	logStep("done", "ok", "Provisioning complete. Waiting for agent heartbeat...")
	h.repo.UpdateStatus(ctx, workerID, models.StatusProvisioned)
	h.repo.AddNotification(ctx, &models.Notification{
		Type:     "success",
		Title:    "Worker Provisioned",
		Message:  ip + " provisioned successfully and waiting for first heartbeat.",
		WorkerID: workerID,
	})
	h.repo.AddAuditLog(ctx, "worker_provisioned", workerID, "Ansible provisioning completed for "+ip)
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

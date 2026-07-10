package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

func HeartbeatReceiver(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			http.Error(w, "missing api key", http.StatusUnauthorized)
			return
		}

		worker, err := repo.GetByAPIKey(r.Context(), apiKey)
		if err != nil {
			http.Error(w, "unknown worker", http.StatusUnauthorized)
			return
		}

		var hb models.Heartbeat
		if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
			http.Error(w, "invalid payload", http.StatusBadRequest)
			return
		}
		hb.WorkerUUID = worker.WorkerUUID

		if err := repo.UpdateHeartbeat(r.Context(), &hb); err != nil {
			http.Error(w, "update failed", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

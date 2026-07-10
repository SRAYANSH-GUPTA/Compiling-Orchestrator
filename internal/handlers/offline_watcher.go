package handlers

import (
	"context"
	"log"
	"time"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

func StartOfflineWatcher(ctx context.Context, repo *repository.WorkerRepo) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			workers, err := repo.List(ctx)
			if err != nil {
				continue
			}
			for _, w := range workers {
				if w.ProvisionStatus != models.StatusOnline {
					continue
				}
				if w.LastHeartbeat != nil && time.Since(*w.LastHeartbeat) > 15*time.Second {
					log.Printf("worker %s (%s) went offline", w.Name, w.IPAddress)
					repo.UpdateStatus(ctx, w.ID, models.StatusOffline)
					repo.AddNotification(ctx, &models.Notification{
						Type:     "error",
						Title:    "Worker Offline",
						Message:  w.Name + " (" + w.IPAddress + ") stopped sending heartbeats.",
						WorkerID: w.ID,
					})
				}
			}
		}
	}
}

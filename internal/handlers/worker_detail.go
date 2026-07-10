package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type workerDetailData struct {
	Page          string
	WorkerCount   int
	Worker        interface{}
	ProvisionLogs interface{}
}

func WorkerDetail(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		worker, err := repo.GetByID(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		logs, _ := repo.GetProvisionLogs(r.Context(), id)
		workers, _ := repo.List(r.Context())
		render(w, "worker_detail.html", &workerDetailData{
			Page:          "workers",
			WorkerCount:   len(workers),
			Worker:        worker,
			ProvisionLogs: logs,
		})
	}
}

func WorkerMetricsAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		worker, err := repo.GetByID(r.Context(), id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		render(w, "worker-metrics", worker)
	}
}

func WorkerProvisionLogAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		logs, _ := repo.GetProvisionLogs(r.Context(), id)
		render(w, "provision-log", logs)
	}
}

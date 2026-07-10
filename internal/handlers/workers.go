package handlers

import (
	"net/http"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type workersData struct {
	Page        string
	WorkerCount int
	Workers     []*models.Worker
}

func WorkersList(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workers, _ := repo.List(r.Context())
		render(w, "workers.html", &workersData{
			Page:        "workers",
			WorkerCount: len(workers),
			Workers:     workers,
		})
	}
}

func WorkersListAPI(repo *repository.WorkerRepo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		workers, _ := repo.List(r.Context())
		render(w, "workers-list", &workersData{Workers: workers})
	}
}

func AddWorkerPage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, "add_worker.html", map[string]interface{}{
			"Page":  "add-worker",
			"Error": "",
		})
	}
}

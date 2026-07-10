package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/auth"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/config"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/db"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/handlers"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
	"github.com/srayansh-gupta/compiling-orchestrator/web"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	if err := handlers.InitTemplates(); err != nil {
		log.Fatalf("templates: %v", err)
	}

	repo := repository.NewWorkerRepo(pool)
	ph := handlers.NewProvisionHandler(repo, cfg)
	ah := handlers.NewActionHandler(repo)

	go handlers.StartOfflineWatcher(ctx, repo)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	staticFS, _ := fs.Sub(web.FS, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	r.Get("/login", handlers.LoginPage)
	r.Post("/login", handlers.LoginPost)
	r.Get("/logout", handlers.Logout)

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware)

		r.Get("/", handlers.Overview(repo))
		r.Get("/workers", handlers.WorkersList(repo))
		r.Get("/workers/add", handlers.AddWorkerPage())
		r.Post("/workers/add", ph.AddWorker)
		r.Post("/workers/update-all", ah.UpdateAll)

		r.Get("/workers/{id}", handlers.WorkerDetail(repo))
		r.Post("/workers/{id}/provision", ph.ProvisionWorker)
		r.Post("/workers/{id}/test-connection", ah.TestConnection)
		r.Post("/workers/{id}/restart-docker", ah.RestartDocker)
		r.Post("/workers/{id}/restart-nomad", ah.RestartNomad)
		r.Post("/workers/{id}/restart-agent", ah.RestartAgent)
		r.Post("/workers/{id}/restart-judge", ah.RestartJudge)
		r.Post("/workers/{id}/drain", ah.Drain)
		r.Post("/workers/{id}/resume", ah.Resume)
		r.Post("/workers/{id}/update", ah.Update)
		r.Post("/workers/{id}/clean-docker", ah.CleanDocker)
		r.Post("/workers/{id}/remove", ah.RemoveWorker)

		r.Get("/api/stats", handlers.StatsAPI(repo))
		r.Get("/api/cluster-health", handlers.ClusterHealthAPI(repo))
		r.Get("/api/service-status", handlers.ServiceStatusAPI(repo))
		r.Get("/api/notifications", handlers.NotificationsAPI(repo))
		r.Get("/api/workers", handlers.WorkersListAPI(repo))
		r.Get("/api/workers/{id}/metrics", handlers.WorkerMetricsAPI(repo))
		r.Get("/api/workers/{id}/provision-log", handlers.WorkerProvisionLogAPI(repo))
	})

	r.Post("/api/heartbeat", handlers.HeartbeatReceiver(repo))

	serveAgentBinary(r)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("dashboard listening on http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-quit
	log.Println("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

func serveAgentBinary(r chi.Router) {
	r.Get("/static/agent/linux-amd64", func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open("bin/agent-linux-amd64")
		if err != nil {
			http.Error(w, "agent binary not built yet", http.StatusNotFound)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=worker-agent")
		http.ServeContent(w, r, "worker-agent", time.Time{}, f)
	})
}

package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
)

type Server struct {
	apiKey     string
	workerUUID string
	ports      []string
}

func NewServer(apiKey, workerUUID string, ports []string) *Server {
	return &Server{apiKey: apiKey, workerUUID: workerUUID, ports: ports}
}

func (s *Server) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.auth(s.health))
	mux.HandleFunc("GET /metrics", s.auth(s.metrics))
	mux.HandleFunc("GET /metrics/prometheus", s.auth(s.metricsPrometheus))
	mux.HandleFunc("POST /restart", s.auth(s.restart))
	mux.HandleFunc("POST /drain", s.auth(s.drain))
	mux.HandleFunc("POST /resume", s.auth(s.resume))
	mux.HandleFunc("POST /update", s.auth(s.update))
	mux.HandleFunc("POST /cleanup", s.auth(s.cleanup))
	mux.HandleFunc("POST /shutdown", s.auth(s.shutdown))

	// Bind the first free candidate port rather than failing outright: a worker
	// may already be running something on the preferred one. The dashboard probes
	// the same list, so it will find whichever we settled on.
	var lastErr error
	for _, port := range s.ports {
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Printf("agent: port %s unavailable (%v), trying next", port, err)
			lastErr = err
			continue
		}
		log.Printf("agent listening on :%s", port)
		return http.Serve(ln, mux)
	}
	return fmt.Errorf("no available port among %v: %w", s.ports, lastErr)
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != s.apiKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	m := CollectMetrics()
	jsonResp(w, map[string]interface{}{
		"status":        "ok",
		"worker_uuid":   s.workerUUID,
		"cpu_usage":     m.CPUUsage,
		"ram_usage":     m.RAMUsage,
		"disk_usage":    m.DiskUsage,
		"docker_status": m.DockerStatus,
		"nomad_status":  m.NomadStatus,
		"judge_status":  m.JudgeStatus,
	})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	m := CollectMetrics()
	jsonResp(w, m)
}

func (s *Server) metricsPrometheus(w http.ResponseWriter, r *http.Request) {
	m := CollectMetrics()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(prometheusMetrics(m)))
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Service string `json:"service"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	if body.Service == "" {
		http.Error(w, "service required", http.StatusBadRequest)
		return
	}
	var cmd *exec.Cmd
	if body.Service == "judge" {
		cmd = exec.Command("sh", "-c", "docker ps -q --filter name=judge | xargs -r docker restart")
	} else {
		cmd = exec.Command("systemctl", "restart", body.Service)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		jsonResp(w, map[string]interface{}{"ok": false, "error": string(out)})
		return
	}
	jsonResp(w, map[string]interface{}{"ok": true})
}

func (s *Server) drain(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]interface{}{"ok": true, "status": "draining"})
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]interface{}{"ok": true, "status": "online"})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	go func() {
		exec.Command("docker", "pull", "judge0/judge0:latest").Run()
		exec.Command("systemctl", "restart", "nomad").Run()
	}()
	jsonResp(w, map[string]interface{}{"ok": true, "message": "update started"})
}

func (s *Server) cleanup(w http.ResponseWriter, r *http.Request) {
	go exec.Command("docker", "system", "prune", "-f").Run()
	jsonResp(w, map[string]interface{}{"ok": true, "message": "cleanup started"})
}

func (s *Server) shutdown(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, map[string]interface{}{"ok": true, "message": "shutdown acknowledged"})
	go exec.Command("systemctl", "stop", "nomad", "docker", "worker-agent").Run()
}

func jsonResp(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

package handlers

import (
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"time"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/web"
)

type pageEntry struct {
	tmpl     *template.Template
	execName string
}

var (
	pageRegistry    map[string]*pageEntry
	partialRegistry map[string]*template.Template
)

func InitTemplates() error {
	sub, err := fs.Sub(web.FS, "templates")
	if err != nil {
		return err
	}
	fns := funcMap()

	type pageConfig struct {
		files    []string
		execName string
	}

	pageDefs := map[string]pageConfig{
		"login.html": {
			files:    []string{"login.html"},
			execName: "login.html",
		},
		"overview.html": {
			files: []string{
				"layout.html", "overview.html",
				"partials/stats.html", "partials/services.html", "partials/notifications.html",
			},
			execName: "overview.html",
		},
		"workers.html": {
			files:    []string{"layout.html", "workers.html", "partials/worker_card.html"},
			execName: "workers.html",
		},
		"add_worker.html": {
			files:    []string{"layout.html", "add_worker.html"},
			execName: "add_worker.html",
		},
		"worker_detail.html": {
			files: []string{
				"layout.html", "worker_detail.html",
				"partials/worker_metrics.html", "partials/provision_log.html",
			},
			execName: "worker_detail.html",
		},
	}

	pageRegistry = make(map[string]*pageEntry, len(pageDefs))
	for name, cfg := range pageDefs {
		t, err := template.New("").Funcs(fns).ParseFS(sub, cfg.files...)
		if err != nil {
			return fmt.Errorf("parse page %s: %w", name, err)
		}
		pageRegistry[name] = &pageEntry{tmpl: t, execName: cfg.execName}
	}

	partialDefs := map[string]string{
		"stats-grid":     "partials/stats.html",
		"service-status": "partials/services.html",
		"notifications":  "partials/notifications.html",
		"workers-list":   "partials/worker_card.html",
		"worker-metrics": "partials/worker_metrics.html",
		"provision-log":  "partials/provision_log.html",
	}

	partialRegistry = make(map[string]*template.Template, len(partialDefs))
	for name, file := range partialDefs {
		t, err := template.New("").Funcs(fns).ParseFS(sub, file)
		if err != nil {
			return fmt.Errorf("parse partial %s: %w", name, err)
		}
		partialRegistry[name] = t
	}
	return nil
}

func render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if e, ok := pageRegistry[name]; ok {
		if err := e.tmpl.ExecuteTemplate(w, e.execName, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if t, ok := partialRegistry[name]; ok {
		if err := t.ExecuteTemplate(w, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	http.Error(w, "template not found: "+name, http.StatusInternalServerError)
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"formatTime": func(t *time.Time) string {
			if t == nil {
				return "never"
			}
			d := time.Since(*t)
			if d < time.Minute {
				return fmt.Sprintf("%ds ago", int(d.Seconds()))
			}
			if d < time.Hour {
				return fmt.Sprintf("%dm ago", int(d.Minutes()))
			}
			return t.Format("Jan 2, 15:04")
		},
		"formatTime2": func(t time.Time) string {
			return t.Format("15:04:05")
		},
		"formatBytes": func(b int64) string {
			if b == 0 {
				return "—"
			}
			const unit = 1024
			if b < unit {
				return fmt.Sprintf("%d B", b)
			}
			div, exp := int64(unit), 0
			for n := b / unit; n >= unit; n /= unit {
				div *= unit
				exp++
			}
			return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
		},
		"cpuColor":      func(v float64) string { return progressColor(v) },
		"ramColor":      func(v float64) string { return progressColor(v) },
		"progressColor": progressColor,
		"notifBadge": func(t string) string {
			switch t {
			case "error", "offline", "down":
				return "badge-failed"
			case "warning", "high-cpu", "high-ram":
				return "badge-draining"
			default:
				return "badge-provisioning"
			}
		},
		"string": func(s models.ProvisionStatus) string { return string(s) },
		"not": func(v interface{}) bool {
			switch val := v.(type) {
			case bool:
				return !val
			case string:
				return val == ""
			case int:
				return val == 0
			case []*models.Worker:
				return len(val) == 0
			case nil:
				return true
			}
			return false
		},
		"or": func(a, b string) string {
			if a != "" {
				return a
			}
			return b
		},
	}
}

func progressColor(v float64) string {
	if v > 85 {
		return "red"
	}
	if v > 65 {
		return "yellow"
	}
	return "green"
}

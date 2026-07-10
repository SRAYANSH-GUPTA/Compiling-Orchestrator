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

var tmpl *template.Template

func InitTemplates() error {
	sub, err := fs.Sub(web.FS, "templates")
	if err != nil {
		return err
	}
	tmpl, err = template.New("").Funcs(funcMap()).ParseFS(sub,
		"layout.html",
		"login.html",
		"overview.html",
		"workers.html",
		"add_worker.html",
		"worker_detail.html",
		"partials/stats.html",
		"partials/services.html",
		"partials/notifications.html",
		"partials/worker_card.html",
		"partials/worker_metrics.html",
		"partials/provision_log.html",
	)
	return err
}

func render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
		"cpuColor": func(v float64) string {
			return progressColor(v)
		},
		"ramColor": func(v float64) string {
			return progressColor(v)
		},
		"progressColor": func(v float64) string {
			return progressColor(v)
		},
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
		"string": func(s models.ProvisionStatus) string {
			return string(s)
		},
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

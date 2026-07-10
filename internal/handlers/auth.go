package handlers

import (
	"net/http"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/auth"
)

func LoginPage(w http.ResponseWriter, r *http.Request) {
	if auth.IsAuthenticated(r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	render(w, "login.html", map[string]interface{}{"Error": ""})
}

func LoginPost(w http.ResponseWriter, r *http.Request) {
	if auth.Login(w, r) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	render(w, "login.html", map[string]interface{}{"Error": "Invalid username or password"})
}

func Logout(w http.ResponseWriter, r *http.Request) {
	auth.Logout(w, r)
	http.Redirect(w, r, "/login", http.StatusFound)
}

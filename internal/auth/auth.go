package auth

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"
)

const (
	Username   = ""
	Password   = ""
	cookieName = "session"
	sessionTTL = 24 * time.Hour
)

type session struct {
	createdAt time.Time
}

var (
	mu       sync.RWMutex
	sessions = map[string]session{}
)

func Login(w http.ResponseWriter, r *http.Request) bool {
	if err := r.ParseForm(); err != nil {
		return false
	}
	if r.FormValue("username") != Username || r.FormValue("password") != Password {
		return false
	}
	token := newToken()
	mu.Lock()
	sessions[token] = session{createdAt: time.Now()}
	mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	return true
}

func Logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(cookieName)
	if err == nil {
		mu.Lock()
		delete(sessions, c.Value)
		mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, MaxAge: -1, Path: "/"})
}

func IsAuthenticated(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return false
	}
	mu.RLock()
	s, ok := sessions[c.Value]
	mu.RUnlock()
	return ok && time.Since(s.createdAt) < sessionTTL
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

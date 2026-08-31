package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
	r.Post("/api/bootstrap", s.bootstrapHandler)
	r.Post("/api/login", s.loginHandler)

	r.Group(func(protected chi.Router) {
		protected.Use(s.requireAuth)
		protected.Post("/api/logout", s.logoutHandler)
		protected.Get("/api/me", s.meHandler)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

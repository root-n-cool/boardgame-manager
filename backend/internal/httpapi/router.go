package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/bgg"
	"boardgames-manager/internal/games"
	"boardgames-manager/internal/settings"
	"boardgames-manager/internal/storage"
	"boardgames-manager/internal/users"
)

type Server struct {
	Users    *users.Store
	Sessions *auth.SessionStore
	Settings *settings.Store
	Games    *games.Store
	Storage  *storage.Store
	BGG      bgg.Client
}

func NewRouter(s *Server) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Get("/api/health", healthHandler)
	r.Get("/api/bootstrap/status", s.bootstrapStatusHandler)
	r.Post("/api/bootstrap", s.bootstrapHandler)
	r.Post("/api/login", s.loginHandler)
	r.Get("/api/games", s.listGamesHandler)
	r.Get("/api/games/{id}", s.getGameHandler)

	r.Group(func(protected chi.Router) {
		protected.Use(s.requireAuth)
		protected.Post("/api/logout", s.logoutHandler)
		protected.Get("/api/me", s.meHandler)
		protected.Get("/api/users", s.listUsersHandler)
		protected.Post("/api/users", s.createUserHandler)
		protected.Delete("/api/users/{id}", s.deleteUserHandler)
		protected.Get("/api/settings", s.getSettingsHandler)
		protected.Put("/api/settings", s.putSettingsHandler)
		protected.Get("/api/games/search", s.searchGamesHandler)
		protected.Post("/api/games", s.createGameHandler)
		protected.Patch("/api/games/{id}", s.updateGameHandler)
		protected.Delete("/api/games/{id}", s.deleteGameHandler)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

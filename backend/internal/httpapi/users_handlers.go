package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

func (s *Server) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, u := range list {
		out = append(out, map[string]any{"id": u.ID, "email": u.Email, "createdAt": u.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	user, err := s.Users.Create(r.Context(), req.Email, hash)
	if err != nil {
		if errors.Is(err, users.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "email": user.Email})
}

func (s *Server) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	count, err := s.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete user")
		return
	}
	if count <= 1 {
		writeError(w, http.StatusConflict, "cannot delete the last remaining user")
		return
	}

	if err := s.Users.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "could not delete user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

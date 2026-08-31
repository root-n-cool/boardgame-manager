package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"boardgames-manager/internal/auth"
)

type bootstrapStatusResponse struct {
	NeedsSetup bool `json:"needsSetup"`
}

func (s *Server) bootstrapStatusHandler(w http.ResponseWriter, r *http.Request) {
	count, err := s.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check setup status")
		return
	}
	writeJSON(w, http.StatusOK, bootstrapStatusResponse{NeedsSetup: count == 0})
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (s *Server) bootstrapHandler(w http.ResponseWriter, r *http.Request) {
	count, err := s.Users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not check setup status")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "setup already completed")
		return
	}

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
		writeError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	if err := s.startSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": user.ID, "email": user.Email})
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, err := auth.GenerateToken()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if err := s.Sessions.Create(r.Context(), userID, auth.HashToken(token), expiresAt); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
	return nil
}

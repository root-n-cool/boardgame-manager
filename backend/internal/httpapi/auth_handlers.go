package httpapi

import (
	"encoding/json"
	"net/http"

	"boardgames-manager/internal/auth"
)

func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	user, err := s.Users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// An admin with an invite still to accept has no password: bcrypt would
	// fail on an empty hash anyway, but saying it here makes the intent
	// readable and does not lean on the library's behaviour.
	if user.Pending() {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !auth.VerifyPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := s.startSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}

func (s *Server) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("session_token"); err == nil {
		if err := s.Sessions.Delete(r.Context(), auth.HashToken(cookie.Value)); err != nil {
			writeError(w, http.StatusInternalServerError, "could not log out")
			return
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) meHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}

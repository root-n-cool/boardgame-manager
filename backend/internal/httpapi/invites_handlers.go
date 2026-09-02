package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/auth"
)

// minPasswordLength is the same minimum the frontend forms ask for
// (minlength=8); the check here is the one that counts.
const minPasswordLength = 8

// getInviteHandler serves the public activation page: it says who the link
// belongs to, so the invitee sees their own email and knows they are on the
// right page. A spent token and an unknown one are indistinguishable: 404.
func (s *Server) getInviteHandler(w http.ResponseWriter, r *http.Request) {
	user, err := s.Users.GetByInviteToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"email": user.Email})
}

type acceptInviteRequest struct {
	Password string `json:"password"`
}

// acceptInviteHandler closes the invite loop: the invitee writes their own
// password, the token dies and the session starts right away — whoever invited
// them never saw that password.
func (s *Server) acceptInviteHandler(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, err := s.Users.GetByInviteToken(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not set the password")
		return
	}

	// Activate fails if the token was spent between the read above and now:
	// two requests on the same link cannot set two different passwords.
	if err := s.Users.Activate(r.Context(), user.ID, hash); err != nil {
		writeError(w, http.StatusNotFound, "invite not found")
		return
	}

	if err := s.startSession(w, r, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not start session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": user.ID, "email": user.Email})
}

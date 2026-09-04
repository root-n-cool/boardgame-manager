package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

// userResponse is the JSON shape of an admin. inviteToken stays a nil
// interface for active admins, so it marshals to null rather than an empty
// string; for pending ones the plaintext token is the point of the feature —
// only an authenticated admin reads it, and they are the one who has to copy
// the link.
func userResponse(u users.User) map[string]any {
	var inviteToken any
	if u.InviteToken != nil {
		inviteToken = *u.InviteToken
	}
	return map[string]any{
		"id":          u.ID,
		"email":       u.Email,
		"createdAt":   u.CreatedAt,
		"pending":     u.Pending(),
		"inviteToken": inviteToken,
	}
}

func (s *Server) listUsersHandler(w http.ResponseWriter, r *http.Request) {
	list, err := s.Users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list users")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, u := range list {
		out = append(out, userResponse(u))
	}
	writeJSON(w, http.StatusOK, out)
}

type inviteUserRequest struct {
	Email string `json:"email"`
}

// createUserHandler no longer accepts a password: whoever invites must not
// know another admin's. It mints an invite link, manda la mail se l'SMTP
// è configurato, e in ogni caso restituisce il token — il bottone "Copia
// link" resta il modo di consegnare l'invito a mano, ed è l'unico su
// un'installazione senza posta.
func (s *Server) createUserHandler(w http.ResponseWriter, r *http.Request) {
	var req inviteUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	token, err := auth.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create invite")
		return
	}

	user, err := s.Users.CreateInvite(r.Context(), email, token)
	if err != nil {
		if errors.Is(err, users.ErrDuplicateEmail) {
			writeError(w, http.StatusConflict, "email already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create invite")
		return
	}

	// La mail è un extra: l'invito è già valido e la risposta non
	// aspetta l'SMTP. mailQueued dice solo se l'invio è stato affidato,
	// non se è arrivato.
	queued := s.mailEnabled(r.Context())
	if queued {
		inviter := "Un amministratore"
		if actor, ok := currentUser(r); ok {
			inviter = actor.Email
		}
		link := inviteURL(s.publicBaseURL(r), token)
		s.sendMailAsync(s.mailSender(r.Context()), inviteMail(email, inviter, link))
	}

	resp := userResponse(user)
	resp["mailQueued"] = queued
	writeJSON(w, http.StatusCreated, resp)
}

func (s *Server) deleteUserHandler(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := s.Users.DeleteIfNotLast(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, users.ErrCannotDeleteLastUser):
			writeError(w, http.StatusConflict, "cannot delete the last remaining user")
		case errors.Is(err, users.ErrNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		default:
			writeError(w, http.StatusInternalServerError, "could not delete user")
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

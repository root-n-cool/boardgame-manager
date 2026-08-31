package httpapi

import (
	"context"
	"net/http"

	"boardgames-manager/internal/auth"
	"boardgames-manager/internal/users"
)

type contextKey string

const userContextKey contextKey = "current_user"

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_token")
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		tokenHash := auth.HashToken(cookie.Value)
		sess, err := s.Sessions.GetValidByTokenHash(r.Context(), tokenHash)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		user, err := s.Users.GetByID(r.Context(), sess.UserID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func currentUser(r *http.Request) (users.User, bool) {
	u, ok := r.Context().Value(userContextKey).(users.User)
	return u, ok
}

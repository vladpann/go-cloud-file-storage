package core_http_middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

type contextKey string

const (
	userIDContextKey    contextKey = "userID"
	sessionIDContextKey contextKey = "sessionID"

	sessionCookieName = "session_id"
)

type SessionsService interface {
	GetSession(ctx context.Context, sessionID uuid.UUID) (domain.Session, error)
}

func Auth(l logger.Interface, sessions SessionsService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			cookie, err := r.Cookie(sessionCookieName)
			if err != nil {
				l.Debug("no session cookie found")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			sessionID, err := uuid.Parse(cookie.Value)
			if err != nil {
				l.Debug("invalid session id format")
				http.Error(w, "invalid session", http.StatusUnauthorized)
				return
			}

			session, err := sessions.GetSession(ctx, sessionID)
			switch {
			case err == nil:
			case errors.Is(err, core_errors.ErrNotFound):
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			case errors.Is(err, context.Canceled):
				return
			default:
				l.Error("auth middleware - GetSession: %v", err)
				http.Error(w, "service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}

			ctx = context.WithValue(ctx, userIDContextKey, session.UserID)
			ctx = context.WithValue(ctx, sessionIDContextKey, sessionID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDContextKey).(uuid.UUID)
	return userID, ok
}

func GetSessionIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	sessionID, ok := ctx.Value(sessionIDContextKey).(uuid.UUID)
	return sessionID, ok
}

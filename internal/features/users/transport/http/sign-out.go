package users_transport_http

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/response"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// SignOut handles user logout by deleting the session
func (h *UsersHTTPHandler) SignOut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersHTTPHandler.SignOut")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	// Get session cookie
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		// Wrap with ErrInvalidArgument to return 400 instead of 500
		err = fmt.Errorf("session cookie not found: %w", core_errors.ErrInvalidArgument)
		span.RecordError(err)
		span.SetStatus(codes.Error, "session cookie not found")
		responseHandler.ErrorResponse(err, "session cookie not found")
		return
	}

	// Parse session ID
	sessionID, err := uuid.Parse(cookie.Value)
	if err != nil {
		// Wrap with ErrInvalidArgument to return 400 instead of 500
		err = fmt.Errorf("invalid session ID: %w", core_errors.ErrInvalidArgument)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid session ID")
		responseHandler.ErrorResponse(err, "invalid session ID")
		return
	}

	span.SetAttributes(attribute.String("session.id", sessionID.String()))

	// Delete session from repository
	if err := h.sessionsService.DeleteSession(ctx, sessionID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to delete session")
		responseHandler.ErrorResponse(err, "failed to delete session")
		return
	}

	span.AddEvent("session deleted")

	// Clear session cookie
	clearCookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.Session.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, clearCookie)

	span.AddEvent("session cookie cleared")
	span.SetStatus(codes.Ok, "sign out successful")

	responseHandler.NoContentResponse()
}

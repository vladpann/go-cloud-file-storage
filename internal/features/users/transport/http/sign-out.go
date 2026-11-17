package users_transport_http

import (
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/response"
)

// SignOut handles user logout by deleting the session
func (h *UsersHTTPHandler) SignOut(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersHTTPHandler.SignOut")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	// Get session ID
	sessionID, ok := core_http_middleware.GetSessionIDFromContext(ctx)
	if !ok {
		// Wrap with ErrInvalidArgument to return 400 instead of 500
		err := fmt.Errorf("invalid session ID: %w", core_errors.ErrInvalidArgument)
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

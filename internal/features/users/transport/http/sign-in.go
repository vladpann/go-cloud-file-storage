package users_transport_http

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_http_request "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/request"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/response"
)

// SignIn handles user authentication
func (h *UsersHTTPHandler) SignIn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersHTTPHandler.SignIn")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	var request SignInRequest
	if err := core_http_request.DecodeAndValidateRequest(r, &request, h.validator); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode and validate request")
		responseHandler.ErrorResponse(err, "failed to decode and validate HTTP request")

		return
	}

	span.SetAttributes(attribute.String("user.username", request.Username))

	// Authenticate user
	username, password := domainFromSignInDTO(request)

	userID, err := h.usersService.SignIn(ctx, username, password)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "authentication failed")
		responseHandler.ErrorResponse(err, "failed to sign in")

		return
	}

	span.SetAttributes(attribute.String("user.id", userID.String()))
	span.AddEvent("user authenticated")

	// Create session
	session, err := h.sessionsService.CreateSession(ctx, userID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create session")
		responseHandler.ErrorResponse(err, "failed to create session")

		return
	}

	span.SetAttributes(
		attribute.String("session.id", session.ID.String()),
		attribute.String("session.expires_at", session.ExpiresAt.Format(time.RFC3339)),
	)
	span.AddEvent("session created")

	// Set session cookie
	cookie := &http.Cookie{ //nolint:gosec // Secure is configurable via SESSION_COOKIE_SECURE (false only for local dev)
		Name:     sessionCookieName,
		Value:    session.ID.String(),
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   h.config.Session.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	span.AddEvent("session cookie set")
	span.SetStatus(codes.Ok, "sign in successful")

	response := signInResponseFromDomain(domain.User{Username: username})

	responseHandler.JSONResponse(response, http.StatusOK)
}

func domainFromSignInDTO(dto SignInRequest) (username, password string) {
	return dto.Username, dto.Password
}

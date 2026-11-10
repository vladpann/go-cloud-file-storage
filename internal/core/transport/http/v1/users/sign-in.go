package core_http_v1_users

import (
	"net/http"
	"time"

	core_http_request "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1/request"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1/response"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)


// SignIn handles user authentication
func (t *UsersTransport) SignIn(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersTransport.SignIn")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(t.l, w)

	var request SignInRequest
	if err := core_http_request.DecodeAndValidateRequest(r, &request, t.validator); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode and validate request")
		responseHandler.ErrorResponse(err, "failed to decode and validate HTTP request")

		return
	}

	span.SetAttributes(attribute.String("user.username", request.Username))

	// Authenticate user
	userID, err := t.usersService.SignIn(ctx, request.Username, request.Password)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "authentication failed")
		responseHandler.ErrorResponse(err, "failed to sign in")

		return
	}

	span.SetAttributes(attribute.String("user.id", userID.String()))
	span.AddEvent("user authenticated")

	// Create session
	session, err := t.sessionsService.CreateSession(ctx, userID)
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
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.ID.String(),
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)

	span.AddEvent("session cookie set")
	span.SetStatus(codes.Ok, "sign in successful")

	response := SignInResponse{
		SessionID: session.ID,
		ExpiresAt: session.ExpiresAt,
	}

	responseHandler.JSONResponse(response, http.StatusOK)
}

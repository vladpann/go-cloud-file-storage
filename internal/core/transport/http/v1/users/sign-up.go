package core_http_v1_users

import (
	"net/http"
	"time"

	core_http_request "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1/request"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1/response"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var tracer = otel.Tracer("github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/v1/users")

const sessionCookieName = "session_id"

// SignUp handles user registration
func (t *UsersTransport) SignUp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersTransport.SignUp")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(t.l, w)

	var request CreateUserRequest
	if err := core_http_request.DecodeAndValidateRequest(r, &request, t.validator); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode and validate request")
		responseHandler.ErrorResponse(err, "failed to decode and validate HTTP request")

		return
	}

	span.SetAttributes(attribute.String("user.username", request.Username))

	username, password, repeatPassword := domainFromDTO(request)

	userDomain, err := t.usersService.SignUp(ctx, username, password, repeatPassword)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create user")
		responseHandler.ErrorResponse(err, "failed to create user")

		return
	}

	span.SetAttributes(
		attribute.String("user.id", userDomain.ID.String()),
		attribute.String("user.username", userDomain.Username),
	)
	span.AddEvent("user created")

	// Create session immediately after registration
	session, err := t.sessionsService.CreateSession(ctx, userDomain.ID)
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
	span.SetStatus(codes.Ok, "user created and logged in successfully")

	response := CreateUserResponse(createUserResponseFromDomain(userDomain, session))

	responseHandler.JSONResponse(response, http.StatusCreated)
}

func domainFromDTO(dto CreateUserRequest) (username string, password string, repeatPassword string) {
	return dto.Username, dto.Password, dto.RepeatPassword
}

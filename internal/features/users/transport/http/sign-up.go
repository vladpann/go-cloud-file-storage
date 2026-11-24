package users_transport_http

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_http_request "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/request"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/response"
)

const sessionCookieName = "session_id"

// SignUp handler user registration
func (h *UsersHTTPHandler) SignUp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersTransport.SignUp")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	var request SignUpRequest
	if err := core_http_request.DecodeAndValidateRequest(r, &request, h.validator); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to decode and validate request")
		responseHandler.ErrorResponse(err, "failed to decode and validate HTTP request")

		return
	}

	span.SetAttributes(attribute.String("user.username", request.Username))

	username, password, repeatPassword := domainFromSignUpDTO(request)

	user, err := h.usersService.SignUp(ctx, username, password, repeatPassword)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create user")
		responseHandler.ErrorResponse(err, "failed to create user")

		return
	}

	span.SetAttributes(
		attribute.String("user.id", user.ID.String()),
		attribute.String("user.username", user.Username),
	)
	span.AddEvent("user created")

	err = h.resourcesService.CreateParentFolder(ctx, user.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create parent folder")
		responseHandler.ErrorResponse(err, "failed to create parent folder")

		return
	}

	span.AddEvent("create parent folder")
	span.SetStatus(codes.Ok, "parent folder created successfully")

	session, err := h.sessionsService.CreateSession(ctx, user.ID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "failed to create session")
		responseHandler.ErrorResponse(err, "failed to create session")

		return
	}

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
	span.SetStatus(codes.Ok, "user created and logged in successfully")

	response := signUpResponseFromDomain(domain.User{Username: username})

	responseHandler.JSONResponse(response, http.StatusCreated)
}

func domainFromSignUpDTO(dto SignUpRequest) (username, password, repeatPassword string) {
	return dto.Username, dto.Password, dto.RepeatPassword
}

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

// Me handles return username
func (h *UsersHTTPHandler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "UsersHTTPHandler.Me")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	// Get user ID
	userID, ok := core_http_middleware.GetUserIDFromContext(ctx)
	if !ok {
		err := fmt.Errorf("invalid user ID: %w", core_errors.ErrInvalidArgument)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid user ID")
		responseHandler.ErrorResponse(err, "invalid user ID")
		return
	}

	span.SetAttributes(attribute.String("user.id", userID.String()))

	username, err := h.usersService.GetUsernameByUserID(ctx, userID)
	if err != nil {
		err := fmt.Errorf("username with user ID: %w", core_errors.ErrInvalidArgument)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid user ID")
		responseHandler.ErrorResponse(err, "invalid user ID")
		return
	}

	span.AddEvent("username found")
	span.SetStatus(codes.Ok, "username successful")

	response := MeResponse{
		Username: username,
	}

	responseHandler.JSONResponse(response, http.StatusOK)
}

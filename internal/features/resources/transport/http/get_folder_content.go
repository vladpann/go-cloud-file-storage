package resources_transport_http

import (
	"errors"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/response"
)

// GetFolderContent handler get folder content
func (h *ResourcesHTTPHandler) GetFolderContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "ResourcesHTTPHandler.GetFolderContent")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	rawPath := r.URL.Query().Get("path")

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

	// Get info folder content
	resources, err := h.resourcesService.GetFolderContent(ctx, userID, rawPath)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidPath):
			err := fmt.Errorf("invalid path: %w", core_errors.ErrInvalidArgument)
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid path")
			responseHandler.ErrorResponse(err, "invalid path")
		case errors.Is(err, domain.ErrResourceNotFound):
			err := fmt.Errorf("folder not found: %w", core_errors.ErrNotFound)
			span.RecordError(err)
			span.SetStatus(codes.Error, "folder not found")
			responseHandler.ErrorResponse(err, "folder not found")
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to get info folder content")
			responseHandler.ErrorResponse(err, "failed to get info folder content")
		}
		return
	}

	span.AddEvent("info folder content got")
	span.SetStatus(codes.Ok, "get info folder content successful")

	response := getFolderContentResponseFromDomain(resources)

	responseHandler.JSONResponse(response, http.StatusOK)
}

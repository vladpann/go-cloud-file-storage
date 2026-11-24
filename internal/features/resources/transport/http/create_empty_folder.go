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

// CreateEmptyFolder handler create empty folder
func (h *ResourcesHTTPHandler) CreateEmptyFolder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "ResourcesHTTPHandler.CreateEmptyFolder")
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

	// Create empty folder
	resource, err := h.resourcesService.CreateEmptyFolder(ctx, userID, rawPath)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidPath):
			err := fmt.Errorf("invalid path: %w", core_errors.ErrInvalidArgument)
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid path")
			responseHandler.ErrorResponse(err, "invalid path")
		case errors.Is(err, domain.ErrResourceNotFound):
			err := fmt.Errorf("parent folder not found: %w", core_errors.ErrNotFound)
			span.RecordError(err)
			span.SetStatus(codes.Error, "parent folder not found")
			responseHandler.ErrorResponse(err, "parent folder not found")
		case errors.Is(err, domain.ErrResourceAlreadyExists):
			err := fmt.Errorf("folder already exists: %w", core_errors.ErrAlreadyExists)
			span.RecordError(err)
			span.SetStatus(codes.Error, "folder already exists")
			responseHandler.ErrorResponse(err, "folder already exists")
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to create empty folder")
			responseHandler.ErrorResponse(err, "failed to create empty folder")
		}
		return
	}

	span.AddEvent("empty folder created")
	span.SetStatus(codes.Ok, "create empty folder successful")

	response := createEmptyFolderResponseFromDomain(resource)

	responseHandler.JSONResponse(response, http.StatusCreated)
}

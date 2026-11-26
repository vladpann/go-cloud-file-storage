package resources_transport_http

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/vladpann/go-cloud-file-storage/internal/core/domain"
	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	core_http_middleware "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/middleware"
	core_http_response "github.com/vladpann/go-cloud-file-storage/internal/core/transport/http/response"
)

const (
	maxUploadMemory   = 32 << 20  // 32 MiB in memory, the rest in temporary files
	maxUploadBodySize = 100 << 20 // 100 MiB max total request body size
)

// UploadResource handles multipart upload of files and folders into target directory.
func (h *ResourcesHTTPHandler) UploadResource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ctx, span := tracer.Start(ctx, "ResourcesHTTPHandler.UploadResource")
	defer span.End()

	responseHandler := core_http_response.NewHTTPResponseHandler(h.l, w)

	rawPath := r.URL.Query().Get("path")

	userID, ok := core_http_middleware.GetUserIDFromContext(ctx)
	if !ok {
		err := fmt.Errorf("invalid user ID: %w", core_errors.ErrInvalidArgument)
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid user ID")
		responseHandler.ErrorResponse(err, "invalid user ID")
		return
	}

	span.SetAttributes(attribute.String("user.id", userID.String()))

	uploads, filesToClose, err := h.parseUploadedFiles(w, r)
	defer h.cleanupUploadedFiles(filesToClose, r.MultipartForm)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		responseHandler.ErrorResponse(err, err.Error())
		return
	}

	resources, err := h.resourcesService.UploadResource(ctx, userID, rawPath, uploads)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidPath), errors.Is(err, domain.ErrInvalidSize):
			err := fmt.Errorf("invalid path, file name or file size: %w", core_errors.ErrInvalidArgument)
			span.RecordError(err)
			span.SetStatus(codes.Error, "invalid path or file size")
			responseHandler.ErrorResponse(err, "invalid path or file size")
		case errors.Is(err, domain.ErrResourceNotFound):
			err := fmt.Errorf("target folder not found: %w", core_errors.ErrNotFound)
			span.RecordError(err)
			span.SetStatus(codes.Error, "target folder not found")
			responseHandler.ErrorResponse(err, "target folder not found")
		case errors.Is(err, domain.ErrResourceAlreadyExists):
			err := fmt.Errorf("file already exists: %w", core_errors.ErrAlreadyExists)
			span.RecordError(err)
			span.SetStatus(codes.Error, "file already exists")
			responseHandler.ErrorResponse(err, "file already exists")
		default:
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to upload resources")
			responseHandler.ErrorResponse(err, "failed to upload resources")
		}
		return
	}

	span.AddEvent("resources uploaded")
	span.SetStatus(codes.Ok, "upload resource successful")

	response := uploadResourceResponseFromDomain(resources)
	responseHandler.JSONResponse(response, http.StatusCreated)
}

func (h *ResourcesHTTPHandler) parseUploadedFiles(
	w http.ResponseWriter,
	r *http.Request,
) ([]domain.UploadFile, []io.Closer, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBodySize)

	//nolint:gosec // G120: request body size is bounded by http.MaxBytesReader above
	if err := r.ParseMultipartForm(maxUploadMemory); err != nil {
		return nil, nil, fmt.Errorf("invalid multipart body: %w", core_errors.ErrInvalidArgument)
	}

	if r.MultipartForm == nil || len(r.MultipartForm.File) == 0 {
		return nil, nil, fmt.Errorf("no files provided: %w", core_errors.ErrInvalidArgument)
	}

	uploads := make([]domain.UploadFile, 0)
	filesToClose := make([]io.Closer, 0)

	for _, headers := range r.MultipartForm.File {
		for _, fh := range headers {
			file, err := fh.Open()
			if err != nil {
				return nil, filesToClose, fmt.Errorf("failed to open uploaded file: %w", err)
			}
			filesToClose = append(filesToClose, file)

			contentType := fh.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			uploads = append(uploads, domain.UploadFile{
				RelativePath: extractFilename(fh),
				Size:         fh.Size,
				ContentType:  contentType,
				Reader:       file,
			})
		}
	}

	if len(uploads) == 0 {
		return nil, filesToClose, fmt.Errorf("no files provided: %w", core_errors.ErrInvalidArgument)
	}

	return uploads, filesToClose, nil
}

func (h *ResourcesHTTPHandler) cleanupUploadedFiles(files []io.Closer, form *multipart.Form) {
	for _, f := range files {
		if err := f.Close(); err != nil {
			h.l.Warn("failed to close uploaded file: %v", err)
		}
	}

	if form != nil {
		if err := form.RemoveAll(); err != nil {
			h.l.Warn("failed to remove multipart temp files: %v", err)
		}
	}
}

func extractFilename(fh *multipart.FileHeader) string {
	disposition := fh.Header.Get("Content-Disposition")
	if disposition != "" {
		if _, params, err := mime.ParseMediaType(disposition); err == nil {
			if rawName, ok := params["filename"]; ok && rawName != "" {
				return strings.TrimPrefix(strings.ReplaceAll(rawName, "\\", "/"), "/")
			}
		}
	}

	return fh.Filename
}

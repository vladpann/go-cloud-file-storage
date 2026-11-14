package core_http_response

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"

	core_errors "github.com/vladpann/go-cloud-file-storage/internal/core/errors"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

type HTTPResponseHandler struct {
	l logger.Interface
	w http.ResponseWriter
}

func NewHTTPResponseHandler(
	l logger.Interface,
	w http.ResponseWriter,
) *HTTPResponseHandler {
	return &HTTPResponseHandler{l: l, w: w}
}

func (h *HTTPResponseHandler) JSONResponse(responseBody any, statusCode int) {
	h.w.Header().Set("Content-Type", "application/json")
	h.w.WriteHeader(statusCode)

	if err := json.NewEncoder(h.w).Encode(responseBody); err != nil {
		h.l.Error("write HTTP response", err)
	}
}

func (h *HTTPResponseHandler) NoContentResponse() {
	h.w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPResponseHandler) ErrorResponse(err error, msg string) {
	var (
		statusCode int
		publicErr  string
	)

	switch {
	case errors.Is(err, core_errors.ErrNotFound):
		statusCode = http.StatusNotFound
		publicErr = core_errors.ErrNotFound.Error()
		h.l.Debug(msg, err)
	case errors.Is(err, core_errors.ErrInvalidCredentials):
		statusCode = http.StatusUnauthorized
		publicErr = core_errors.ErrInvalidCredentials.Error()
		h.l.Warn(msg, err)
	case errors.Is(err, core_errors.ErrUserAlreadyExists):
		statusCode = http.StatusConflict
		publicErr = core_errors.ErrUserAlreadyExists.Error()
		h.l.Warn(msg, err)
	case errors.Is(err, core_errors.ErrInvalidArgument):
		statusCode = http.StatusBadRequest
		publicErr = core_errors.ErrInvalidArgument.Error()
		h.l.Warn(msg, err)
	default:
		statusCode = http.StatusInternalServerError
		publicErr = ""
		h.l.Error(msg, err)
	}

	h.errorResponse(statusCode, publicErr, msg)
}

func (h *HTTPResponseHandler) PanicResponse(p any, msg string) {
	h.l.Error(
		fmt.Sprintf("%s: panic=%v, stack=%s", msg, p, debug.Stack()),
		nil,
	)

	h.errorResponse(http.StatusInternalServerError, "", msg)
}

func (h *HTTPResponseHandler) errorResponse(
	statusCode int,
	publicErr string,
	msg string,
) {
	h.JSONResponse(
		ErrorResponse{
			Error:   publicErr,
			Message: msg,
		},
		statusCode,
	)
}

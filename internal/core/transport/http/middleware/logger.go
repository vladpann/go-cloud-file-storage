package core_http_middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

func buildRequestMessage(r *http.Request, status, size int) string {
	var result strings.Builder

	result.WriteString(r.RemoteAddr)
	result.WriteString(" - ")
	result.WriteString(r.Method)
	result.WriteString(" ")
	result.WriteString(r.RequestURI)
	result.WriteString(" - ")
	result.WriteString(strconv.Itoa(status))
	result.WriteString(" ")
	result.WriteString(strconv.Itoa(size))

	return result.String()
}

func Logger(l logger.Interface) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			// Use context-aware logging to include request_id
			ctx := r.Context()
			l.InfoContext(ctx, "%s", buildRequestMessage(r, status, ww.BytesWritten()))
		})
	}
}

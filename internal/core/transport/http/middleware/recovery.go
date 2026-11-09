package core_http_middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/vladpann/go-cloud-file-storage/pkg/logger"
)

func buildPanicMessage(r *http.Request, err any) string {
	var result strings.Builder

	result.WriteString(r.RemoteAddr)
	result.WriteString(" - ")
	result.WriteString(r.Method)
	result.WriteString(" ")
	result.WriteString(r.RequestURI)
	result.WriteString(" PANIC DETECTED: ")
	fmt.Fprintf(&result, "%v\n%s\n", err, debug.Stack())

	return result.String()
}

func Recovery(l logger.Interface) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				if rec == http.ErrAbortHandler {
					panic(rec)
				}

				l.Error(buildPanicMessage(r, rec))

				w.WriteHeader(http.StatusInternalServerError)
			}()

			next.ServeHTTP(w, r)
		})
	}
}

package core_http_middleware

import (
	"net/http"
	"slices"
)

type Middleware func(http.Handler) http.Handler

// ChainMiddleware applies a slice of middleware to a handler in reverse order.
// This ensures that the first middleware in the slice is executed first.
//
// Example:
//
//	handler := ChainMiddleware(
//		finalHandler,
//		RequestID(),      // executes first
//		Logger(l),        // executes second
//		Recovery(l),      // executes third (wraps the handler)
//	)
//
// The middleware chain wraps the handler from right to left (last to first),
// so when a request comes in, it executes from left to right (first to last).
func ChainMiddleware(
	h http.Handler,
	m ...Middleware,
) http.Handler {
	if len(m) == 0 {
		return h
	}

	for _, v := range slices.Backward(m) {
		h = v(h)
	}

	return h
}

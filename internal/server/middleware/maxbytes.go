package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

// DefaultMaxRequestSize is the default limit for request bodies (5 MiB).
const DefaultMaxRequestSize int64 = 5 << 20

// MaxRequestBody rejects request bodies larger than limit bytes with
// 413 Request Entity Too Large. A declared Content-Length over the limit is
// refused before the body is read; a body of unknown length (chunked) is read
// up to the limit first, so the handler never sees more than limit bytes and
// an oversized body gets the same 413.
func MaxRequestBody(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			if r.ContentLength < 0 {
				body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
				if err != nil {
					var tooLarge *http.MaxBytesError
					if errors.As(err, &tooLarge) {
						http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
						return
					}
					http.Error(w, "failed to read request body", http.StatusBadRequest)
					return
				}
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
			}
			next.ServeHTTP(w, r)
		})
	}
}

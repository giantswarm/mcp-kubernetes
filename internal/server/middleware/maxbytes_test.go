package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// chunkedReader hides the length of its content, so httptest.NewRequest
// leaves ContentLength unset (-1), as for a chunked request.
type chunkedReader struct{ io.Reader }

func TestMaxRequestBody(t *testing.T) {
	const limit = 16
	var received string
	handler := MaxRequestBody(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		received = string(body)
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name       string
		body       io.Reader
		wantStatus int
		wantBody   string
	}{
		{name: "no body", body: nil, wantStatus: http.StatusOK},
		{name: "under the limit", body: strings.NewReader("small"), wantStatus: http.StatusOK, wantBody: "small"},
		{name: "exactly the limit", body: strings.NewReader(strings.Repeat("a", limit)), wantStatus: http.StatusOK, wantBody: strings.Repeat("a", limit)},
		{name: "declared length over the limit", body: strings.NewReader(strings.Repeat("a", limit+1)), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "chunked under the limit", body: chunkedReader{strings.NewReader("small")}, wantStatus: http.StatusOK, wantBody: "small"},
		{name: "chunked exactly the limit", body: chunkedReader{strings.NewReader(strings.Repeat("a", limit))}, wantStatus: http.StatusOK, wantBody: strings.Repeat("a", limit)},
		{name: "chunked over the limit", body: chunkedReader{strings.NewReader(strings.Repeat("a", limit+1))}, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			received = ""
			req := httptest.NewRequest(http.MethodPost, "/mcp", tt.body)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, received)
		})
	}
}

func TestMaxRequestBodyReadError(t *testing.T) {
	handler := MaxRequestBody(16)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/mcp", chunkedReader{errReader{}})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

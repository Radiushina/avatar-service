package logger

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestLoggingMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		url        string
		handle     func(http.ResponseWriter, *http.Request)
		wantStatus int
		wantSize   int
		wantURI    string
	}{
		{
			name:   "implicit 200",
			method: http.MethodGet,
			url:    "http://example.test/some/path",
			handle: func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, "ok")
			},
			wantStatus: http.StatusOK,
			wantSize:   2,
			wantURI:    "/some/path",
		},
		{
			name:   "explicit write header",
			method: http.MethodPost,
			url:    "http://example.test/value",
			handle: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
			wantStatus: http.StatusNoContent,
			wantURI:    "/value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			core, logs := observer.New(zap.InfoLevel)
			h := LoggingMiddleware(zap.New(core), http.HandlerFunc(tc.handle))
			req := httptest.NewRequest(tc.method, tc.url, nil)
			req.Header.Set("Authorization", "Bearer super-secret-token")
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			entries := logs.All()
			require.Len(t, entries, 1)
			require.Equal(t, "HTTP request", entries[0].Message)

			fields := map[string]any{}
			for _, f := range entries[0].Context {
				require.NotEqual(t, "authorization", f.Key)
				require.NotContains(t, f.String, "super-secret-token")
				require.NotContains(t, f.String, "Bearer ")
				switch f.Key {
				case "status", "response_size":
					fields[f.Key] = int(f.Integer)
				case "method", "uri":
					fields[f.Key] = f.String
				}
			}
			require.Equal(t, tc.wantStatus, fields["status"])
			require.Equal(t, tc.method, fields["method"])
			require.Equal(t, tc.wantURI, fields["uri"])
			if tc.wantSize > 0 {
				require.Equal(t, tc.wantSize, fields["response_size"])
			}
		})
	}
}

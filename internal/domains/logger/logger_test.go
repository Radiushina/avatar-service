package logger_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Radiushina/avatar-service/internal/domains/logger"
	"github.com/stretchr/testify/require"
	sdklog "go.opentelemetry.io/otel/sdk/log"
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
			h := logger.LoggingMiddleware(zap.New(core), http.HandlerFunc(tc.handle))
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

func TestNewSlogLevel(t *testing.T) {
	t.Parallel()

	sl, err := logger.NewSlog("error")
	require.NoError(t, err)
	require.False(t, sl.Enabled(context.Background(), slog.LevelInfo))
	require.True(t, sl.Enabled(context.Background(), slog.LevelError))
}

func TestAttachExportDropsVerboseLogs(t *testing.T) {
	t.Parallel()

	exp := &captureExp{}
	provider := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(exp)))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	zl, err := logger.New("error")
	require.NoError(t, err)
	sl, err := logger.NewSlog("error")
	require.NoError(t, err)
	zl, sl, err = logger.AttachExport("error", zl, sl, provider)
	require.NoError(t, err)

	sl.Info("skip-info")
	zl.Info("skip-zap")
	sl.Error("keep-error")
	zl.Error("keep-zap")

	require.Equal(t, 2, exp.n)
}

type captureExp struct {
	n int
}

func (c *captureExp) Export(_ context.Context, records []sdklog.Record) error {
	c.n += len(records)
	return nil
}

func (*captureExp) Shutdown(context.Context) error { return nil }

func (*captureExp) ForceFlush(context.Context) error { return nil }

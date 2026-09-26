package logger

import (
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"
)

// New builds a zap production logger configured with the provided level.
func New(level string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, err
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	cfg.DisableStacktrace = true
	zl, err := cfg.Build()
	if err != nil {
		return nil, err
	}
	return zl, nil
}

func Exit(log *zap.Logger, msg string, err error) {
	if log != nil {
		log.Error(msg, zap.Error(err))
		_ = log.Sync()
	}
	os.Exit(1)
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (lw *loggingResponseWriter) WriteHeader(code int) {
	if lw.wroteHeader {
		return
	}
	lw.status = code
	lw.wroteHeader = true
	lw.ResponseWriter.WriteHeader(code)
}

func (lw *loggingResponseWriter) Write(b []byte) (int, error) {
	if !lw.wroteHeader {
		lw.WriteHeader(http.StatusOK)
	}
	n, err := lw.ResponseWriter.Write(b)
	lw.bytes += n
	return n, err
}

// LoggingMiddleware logs request details (URI, method, duration)
// and response details (status code, response size).
func LoggingMiddleware(log *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(lw, r)
		uri := r.URL.RequestURI()
		log.Info("HTTP request",
			zap.String("uri", uri),
			zap.String("method", r.Method),
			zap.Duration("duration", time.Since(start)),
			zap.Int("status", lw.status),
			zap.Int("response_size", lw.bytes),
		)
	})
}

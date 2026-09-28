package logger

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// New builds a zap production logger configured with the provided level.
func New(level string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	cfg.DisableStacktrace = true
	zl, err := cfg.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}
	return zl, nil
}

// LogError writes err and flushes buffered logs. The caller exits the process.
func LogError(log *zap.Logger, msg string, err error) {
	if log == nil {
		return
	}
	log.Error(msg, zap.Error(err))
	Sync(log)
}

// Sync flushes buffered log entries. fsync on stdout and stderr fails on
// Linux and macOS; those errors are ignored.
func Sync(log *zap.Logger) {
	if log == nil {
		return
	}
	err := log.Sync()
	if err == nil || errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTTY) || errors.Is(err, syscall.ENOTSUP) {
		return
	}
	if _, writeErr := fmt.Fprintf(os.Stderr, "sync logger: %v\n", err); writeErr != nil {
		return
	}
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
	if err != nil {
		return n, fmt.Errorf("write response: %w", err)
	}
	return n, nil
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

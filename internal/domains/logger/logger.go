package logger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

// NewSlog builds a JSON logger. Level follows LOG_LEVEL: debug, info, warn, error.
func NewSlog(level string) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})), nil
}

// AttachExport keeps stdout logs and also sends them to the OTLP logger provider.
// Records below level are not exported.
func AttachExport(level string, zl *zap.Logger, sl *slog.Logger, lp otellog.LoggerProvider) (*zap.Logger, *slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, nil, fmt.Errorf("parse log level: %w", err)
	}
	zapMin, err := zapcore.ParseLevel(level)
	if err != nil {
		return nil, nil, fmt.Errorf("parse log level: %w", err)
	}

	exportedZap := minCore{
		min:  zapMin,
		Core: otelzap.NewCore("avatar-service", otelzap.WithLoggerProvider(lp)),
	}
	zl = zl.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, exportedZap)
	}))
	sl = slog.New(tee{hs: []slog.Handler{
		sl.Handler(),
		minHandler{min: lvl, next: otelslog.NewHandler("avatar-service", otelslog.WithLoggerProvider(lp))},
	}})
	return zl, sl, nil
}

type minCore struct {
	min zapcore.Level
	zapcore.Core
}

func (m minCore) Enabled(level zapcore.Level) bool {
	return level >= m.min && m.Core.Enabled(level)
}

func (m minCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if ent.Level < m.min {
		return ce
	}
	return m.Core.Check(ent, ce)
}

func (m minCore) With(fields []zapcore.Field) zapcore.Core {
	return minCore{min: m.min, Core: m.Core.With(fields)}
}

type minHandler struct {
	min  slog.Level
	next slog.Handler
}

func (h minHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.min
}

func (h minHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level < h.min {
		return nil
	}
	if err := h.next.Handle(ctx, record); err != nil {
		return fmt.Errorf("export log: %w", err)
	}
	return nil
}

func (h minHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return minHandler{min: h.min, next: h.next.WithAttrs(attrs)}
}

func (h minHandler) WithGroup(name string) slog.Handler {
	return minHandler{min: h.min, next: h.next.WithGroup(name)}
}

type tee struct {
	hs []slog.Handler
}

func (t tee) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range t.hs {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (t tee) Handle(ctx context.Context, record slog.Record) error {
	var err error
	for _, h := range t.hs {
		if !h.Enabled(ctx, record.Level) {
			continue
		}
		if handleErr := h.Handle(ctx, record.Clone()); handleErr != nil {
			err = errors.Join(err, handleErr)
		}
	}
	return err
}

func (t tee) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(t.hs))
	for i, h := range t.hs {
		next[i] = h.WithAttrs(attrs)
	}
	return tee{hs: next}
}

func (t tee) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(t.hs))
	for i, h := range t.hs {
		next[i] = h.WithGroup(name)
	}
	return tee{hs: next}
}

// LogError writes err and flushes buffered logs. The caller exits the process.
func LogError(log *zap.Logger, msg string, err error) {
	if log == nil {
		return
	}
	log.Error(msg, zap.Error(err))
	Sync(log)
}

func LogStartupErr(msg string, err error) {
	log, logErr := New("error")
	if logErr != nil {
		return
	}
	LogError(log, msg, err)
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
		log.Info("HTTP request",
			zap.String("uri", r.URL.RequestURI()),
			zap.String("method", r.Method),
			zap.Duration("duration", time.Since(start)),
			zap.Int("status", lw.status),
			zap.Int("response_size", lw.bytes),
			zap.String("trace_id", trace.SpanFromContext(r.Context()).SpanContext().TraceID().String()),
		)
	})
}

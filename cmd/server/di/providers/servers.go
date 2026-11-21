package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/Radiushina/avatar-service/internal/config"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/Radiushina/avatar-service/internal/domains/health"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
	"github.com/Radiushina/avatar-service/internal/domains/webui"
	"github.com/Radiushina/avatar-service/internal/telemetry"
	"github.com/labstack/echo/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
)

type Servers struct {
	http *http.Server
	log  *zap.Logger
}

func NewHTTPServer(
	cfg *config.Config,
	log *zap.Logger,
	avatar avatars.ServiceProvider,
	dbCheck health.DBHealthCheckProvider,
	s3 health.S3HealthCheckProvider,
	broker health.BrokerHealthCheckProvider,
) (*http.Server, error) {
	e := echo.New()
	e.Use(httpMetrics)
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	avatars.NewAvatarRouter(e.Group("/api/v1"), avatar, log)
	health.NewHealthCheckRouter(e.Group(""), log, dbCheck, s3, broker)
	if err := webui.NewRouter(e.Group("/web"), avatar, "web/templates", log); err != nil {
		return nil, fmt.Errorf("web router: %w", err)
	}
	logged := applogger.LoggingMiddleware(log, e)
	traced := otelhttp.NewHandler(logged, "avatars-service",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
	return &http.Server{
		Addr:              cfg.Server.HTTP.Address,
		Handler:           traced,
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
}

func httpMetrics(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		start := time.Now()
		err := next(c)
		status := http.StatusOK
		if resp, ok := c.Response().(*echo.Response); ok && resp.Status != 0 {
			status = resp.Status
		} else if err != nil {
			status = http.StatusInternalServerError
		}
		route := c.Path()
		if route == "" {
			route = "unknown"
		}
		telemetry.HTTPRequests.WithLabelValues(c.Request().Method, route, strconv.Itoa(status)).Inc()
		telemetry.HTTPDuration.WithLabelValues(c.Request().Method, route).Observe(time.Since(start).Seconds())
		return err
	}
}

func NewServers(httpServer *http.Server, log *zap.Logger) *Servers {
	return &Servers{http: httpServer, log: log}
}

func (s *Servers) Start(ctx context.Context) error {
	errCh := make(chan error, 2)
	go func() {
		s.log.Info("starting HTTP server", zap.String("addr", s.http.Addr))
		err := s.http.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		if err != nil {
			s.log.Error("listen", zap.Error(err))
			return err
		}
		return nil
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.http.Shutdown(shutCtx); err != nil {
			s.log.Error("http shutdown", zap.Error(err))
			return fmt.Errorf("http shutdown: %w", err)
		}

		s.log.Info("HTTP server stopped")
		<-errCh
		return nil
	}
}

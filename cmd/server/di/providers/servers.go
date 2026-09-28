package providers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Radiushina/avatar-service/internal/config"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
	"github.com/Radiushina/avatar-service/internal/domains/webui"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type Servers struct {
	http *http.Server
	log  *zap.Logger
}

func NewHTTPServer(cfg *config.Config, log *zap.Logger, avatar avatars.ServiceProvider) (*http.Server, error) {
	e := echo.New()
	avatars.NewAvatarRouter(e.Group("/api/v1"), avatar, log)
	if err := webui.NewRouter(e.Group("/web"), avatar, "web/templates", log); err != nil {
		return nil, fmt.Errorf("web router: %w", err)
	}
	return &http.Server{
		Addr:              cfg.Server.HTTP.Address,
		Handler:           applogger.LoggingMiddleware(log, e),
		ReadHeaderTimeout: 5 * time.Second,
	}, nil
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

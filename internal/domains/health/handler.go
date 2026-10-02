package health

import (
	"context"
	"net/http"

	"github.com/Radiushina/avatar-service/internal/entity"
	"github.com/labstack/echo/v5"
	"go.uber.org/zap"
)

type (
	healthCheckRouter struct {
		db     DBHealthCheckProvider
		s3     S3HealthCheckProvider
		broker BrokerHealthCheckProvider
		log    *zap.Logger
	}
	DBHealthCheckProvider interface {
		Ping(ctx context.Context) error
	}

	S3HealthCheckProvider interface {
		Ping(ctx context.Context) error
	}

	BrokerHealthCheckProvider interface {
		Ping(ctx context.Context) error
	}
)

func NewHealthCheckRouter(
	group *echo.Group,
	log *zap.Logger,
	db DBHealthCheckProvider,
	s3 S3HealthCheckProvider,
	broker BrokerHealthCheckProvider,
) {
	r := healthCheckRouter{
		db:     db,
		s3:     s3,
		broker: broker,
		log:    log,
	}
	group.GET("/health", r.check)
}

func (h *healthCheckRouter) check(c *echo.Context) error {
	ctx := c.Request().Context()
	components := entity.Components{
		Database: h.status(ctx, "database", h.db),
		S3:       h.status(ctx, "s3", h.s3),
		Broker:   h.status(ctx, "broker", h.broker),
	}
	status := "ok"
	code := http.StatusOK
	if components.Database != string(entity.Up) || components.S3 != string(entity.Up) || components.Broker != string(entity.Up) {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	return c.JSON(code, entity.HealthCheck{
		Status:     status,
		Components: components,
	})
}

func (h *healthCheckRouter) status(
	ctx context.Context,
	name string,
	p interface {
		Ping(ctx context.Context) error
	},
) string {
	if p == nil {
		return name + " not configured"
	}
	err := p.Ping(ctx)
	if err != nil {
		h.log.Error(name+" ping", zap.Error(err))
		return string(entity.Down)
	}
	return string(entity.Up)
}

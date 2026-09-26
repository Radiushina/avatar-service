package di

import (
	"context"

	"github.com/Radiushina/avatar-service/cmd/server/di/providers"
	"github.com/Radiushina/avatar-service/internal/config"
	"go.uber.org/zap"
)

type App struct {
	cfg    *config.Config
	server *providers.Servers
	Log    *zap.Logger
}

func (a *App) Run(ctx context.Context) error {
	return a.server.Start(ctx)
}

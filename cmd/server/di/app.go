package di

import (
	"context"
	"fmt"

	"github.com/Radiushina/avatar-service/cmd/server/di/providers"
	"github.com/Radiushina/avatar-service/internal/config"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type App struct {
	cfg    *config.Config
	server *providers.Servers
	Log    *zap.Logger
	pool   *pgxpool.Pool
	repo   *avatars.Repo
}

func (a *App) CollectRuntimeMetrics(ctx context.Context) {
	providers.CollectRuntimeMetrics(ctx, a.pool, a.repo, a.Log)
}

func (a *App) Run(ctx context.Context) error {
	if err := a.server.Start(ctx); err != nil {
		return fmt.Errorf("start server: %w", err)
	}
	return nil
}

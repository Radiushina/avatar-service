package providers

import (
	"context"
	"fmt"

	"github.com/Radiushina/avatar-service/internal/config"
	"github.com/Radiushina/avatar-service/migrations"
	"github.com/Radiushina/avatar-service/pkg/pgmigrator"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func NewPostgres(ctx context.Context, cfg *config.Config, log *zap.Logger) (*pgxpool.Pool, func(), error) {
	if err := pgmigrator.MigrateFromEmbeddedFS(migrations.Postgres, "postgres", cfg.Database.DSN, log); err != nil {
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("postgres ping: %w", err)
	}

	return pool, pool.Close, nil
}

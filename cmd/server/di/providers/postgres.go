package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/Radiushina/avatar-service/internal/config"
	"github.com/Radiushina/avatar-service/internal/telemetry"
	"github.com/Radiushina/avatar-service/migrations"
	"github.com/Radiushina/avatar-service/pkg/pgmigrator"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Postgres struct {
	client *pgxpool.Pool
	log    *zap.Logger
}

func NewPostgres(ctx context.Context, cfg *config.Config, log *zap.Logger) (*pgxpool.Pool, func(), error) {
	if err := pgmigrator.MigrateFromEmbeddedFS(migrations.Postgres, "postgres", cfg.Database.DSN, log); err != nil {
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	cfgPool, err := pgxpool.ParseConfig(cfg.Database.DSN)
	if err != nil {
		return nil, nil, fmt.Errorf("postgres config: %w", err)
	}
	cfgPool.ConnConfig.Tracer = otelpgx.NewTracer()
	pool, err := pgxpool.NewWithConfig(ctx, cfgPool)

	if err != nil {
		return nil, nil, fmt.Errorf("postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("postgres ping: %w", err)
	}

	go collectRuntimeMetrics(ctx, pool)
	return pool, pool.Close, nil
}

func collectRuntimeMetrics(ctx context.Context, pool *pgxpool.Pool) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		sampleRuntimeMetrics(pool)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func sampleRuntimeMetrics(pool *pgxpool.Pool) {
	stat := pool.Stat()
	telemetry.DBConns.WithLabelValues("acquired").Set(float64(stat.AcquiredConns()))
	telemetry.DBConns.WithLabelValues("idle").Set(float64(stat.IdleConns()))
	telemetry.DBConns.WithLabelValues("max").Set(float64(stat.MaxConns()))
}

func NewDBHealth(pool *pgxpool.Pool, log *zap.Logger) *Postgres {
	return &Postgres{client: pool, log: log}
}

func (p *Postgres) Ping(ctx context.Context) error {
	if p == nil || p.client == nil {
		return fmt.Errorf("database not configured")
	}
	if err := p.client.Ping(ctx); err != nil {
		if p.log != nil {
			p.log.Error("db ping", zap.Error(err))
		}
		return fmt.Errorf("database unavailable: %w", err)
	}
	return nil
}

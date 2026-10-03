package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Radiushina/avatar-service/cmd/server/di/providers"
	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
	"github.com/Radiushina/avatar-service/internal/worker"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := providers.NewConfig()
	if err != nil {
		applogger.LogStartupErr("load config", err)
		return fmt.Errorf("load config: %w", err)
	}
	log, stopLog, err := providers.NewLogger(cfg)
	if err != nil {
		applogger.LogStartupErr("logger", err)
		return fmt.Errorf("logger: %w", err)
	}
	defer stopLog()

	pool, stopDB, err := providers.NewPostgres(ctx, cfg, log)
	if err != nil {
		applogger.LogError(log, "postgres", err)
		return fmt.Errorf("postgres: %w", err)
	}
	defer stopDB()

	client, err := broker.Dial(cfg.RabbitMQ.URL, log)
	if err != nil {
		applogger.LogError(log, "rabbitmq", err)
		return fmt.Errorf("rabbitmq: %w", err)
	}
	defer client.Close()

	publisher, err := client.Publisher(ctx)
	if err != nil {
		applogger.LogError(log, "rabbitmq publisher", err)
		return fmt.Errorf("rabbitmq publisher: %w", err)
	}

	store := providers.NewS3Store(cfg)
	w := worker.New(avatars.NewAvatarRepo(pool, store), store, worker.NewResizer(), client, publisher, log)
	if err := w.Run(ctx); err != nil {
		applogger.LogError(log, "run worker", err)
		return fmt.Errorf("run worker: %w", err)
	}
	return nil
}

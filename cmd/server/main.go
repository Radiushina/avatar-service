package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Radiushina/avatar-service/cmd/server/di"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
	"github.com/Radiushina/avatar-service/internal/telemetry"
)

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	stopTelemetry, err := telemetry.InitTracerProvider(context.Background())
	if err != nil {
		applogger.LogStartupErr("tracer", err)
		return err
	}
	defer stopTelemetry()

	stopLogs, err := telemetry.InitLogProvider(context.Background())
	if err != nil {
		applogger.LogStartupErr("logs", err)
		return fmt.Errorf("logs: %w", err)
	}
	defer stopLogs()

	app, cleanup, err := di.InjectApp(ctx)
	if err != nil {
		applogger.LogStartupErr("inject app", err)
		return fmt.Errorf("inject app: %w", err)
	}
	defer cleanup()

	if err := app.Run(ctx); err != nil {
		applogger.LogError(app.Log, "run app", err)
		return fmt.Errorf("run app: %w", err)
	}
	return nil
}

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Radiushina/avatar-service/cmd/server/di"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app, cleanup, err := di.InjectApp(ctx)
	if err != nil {
		fatal("inject app", err)
	}
	defer cleanup()

	if err := app.Run(ctx); err != nil {
		applogger.Exit(app.Log, "run app", err)
	}
}

func fatal(msg string, err error) {
	log, logErr := applogger.New("error")
	if logErr != nil {
		os.Exit(1)
	}
	applogger.Exit(log, msg, err)
}

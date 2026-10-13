package providers

import (
	"fmt"
	"log/slog"

	"github.com/Radiushina/avatar-service/internal/config"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
	"github.com/Radiushina/avatar-service/internal/telemetry"
	"go.uber.org/zap"
)

func NewLogger(cfg *config.Config) (*zap.Logger, func(), error) {
	zl, err := applogger.New(cfg.Log.Level)
	if err != nil {
		return nil, nil, fmt.Errorf("logger: %w", err)
	}
	sl, err := applogger.NewSlog(cfg.Log.Level)
	if err != nil {
		return nil, nil, fmt.Errorf("slog: %w", err)
	}
	if lp := telemetry.LoggerProvider(); lp != nil {
		zl, sl, err = applogger.AttachExport(cfg.Log.Level, zl, sl, lp)
		if err != nil {
			return nil, nil, fmt.Errorf("log export: %w", err)
		}
	}
	slog.SetDefault(sl)
	return zl, func() { applogger.Sync(zl) }, nil
}

package providers

import (
	"fmt"

	"github.com/Radiushina/avatar-service/internal/config"
	applogger "github.com/Radiushina/avatar-service/internal/domains/logger"
	"go.uber.org/zap"
)

func NewLogger(cfg *config.Config) (*zap.Logger, func(), error) {
	zl, err := applogger.New(cfg.Log.Level)
	if err != nil {
		return nil, nil, fmt.Errorf("logger: %w", err)
	}
	return zl, func() { _ = zl.Sync() }, nil
}

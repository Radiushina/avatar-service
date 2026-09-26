package providers

import (
	"fmt"

	"github.com/Radiushina/avatar-service/internal/config"
)

func NewConfig() (*config.Config, error) {
	cfg, err := config.NewLoader("").Load()
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation: %w", err)
	}
	return cfg, nil
}

package providers

import (
	"context"
	"fmt"

	"github.com/Radiushina/avatar-service/internal/broker"
	"github.com/Radiushina/avatar-service/internal/config"
	"go.uber.org/zap"
)

func NewRabbit(cfg *config.Config, log *zap.Logger) (*broker.Client, func(), error) {
	client, err := broker.Dial(cfg.RabbitMQ.URL, log)
	if err != nil {
		return nil, nil, fmt.Errorf("rabbitmq: %w", err)
	}
	return client, client.Close, nil
}

func NewPublisher(ctx context.Context, client *broker.Client) (*broker.Publisher, error) {
	publisher, err := client.Publisher(ctx)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq publisher: %w", err)
	}
	return publisher, nil
}

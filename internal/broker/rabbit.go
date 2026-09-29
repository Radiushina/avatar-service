package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"
)

type Client struct {
	conn *amqp.Connection
	log  *zap.Logger
}

type Publisher struct {
	ch *amqp.Channel
}

func Dial(url string, log *zap.Logger) (*Client, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &Client{conn: conn, log: log}, nil
}

func (c *Client) Close() {
	if c == nil || c.conn == nil {
		return
	}
	if err := c.conn.Close(); err != nil {
		c.log.Error("close rabbitmq", zap.Error(err))
	}
}

func (c *Client) Publisher(ctx context.Context) (*Publisher, error) {
	ch, err := c.channel(ctx)
	if err != nil {
		return nil, err
	}
	return &Publisher{ch: ch}, nil
}

func (c *Client) channel(ctx context.Context) (*amqp.Channel, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}
	if err := declare(ch); err != nil {
		if closeErr := ch.Close(); closeErr != nil {
			c.log.Error("close rabbitmq channel", zap.Error(closeErr))
		}
		return nil, err
	}
	return ch, nil
}

func declare(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}
	for _, key := range []string{KeyUploaded, KeyDeleted} {
		if _, err := ch.QueueDeclare(key, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare queue %s: %w", key, err)
		}
		if err := ch.QueueBind(key, key, Exchange, false, nil); err != nil {
			return fmt.Errorf("bind queue %s: %w", key, err)
		}
	}
	return nil
}

func (p *Publisher) Publish(ctx context.Context, exchange, routingKey string, event any) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	err = p.ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID(event),
		Timestamp:    time.Now().UTC(),
		Body:         body,
	})
	if err != nil {
		return fmt.Errorf("publish %s: %w", routingKey, err)
	}
	return nil
}

func messageID(event any) string {
	switch e := event.(type) {
	case AvatarUploadEvent:
		return withID(KeyUploaded, e.AvatarID)
	case AvatarDeleteEvent:
		return withID(KeyDeleted, e.AvatarID)
	case AvatarProcessEvent:
		return withID("avatar.process", e.AvatarID)
	default:
		return uuid.NewString()
	}
}

func withID(prefix, id string) string {
	if id == "" {
		return uuid.NewString()
	}
	return prefix + ":" + id
}

func (c *Client) Listen(ctx context.Context, queue string, handle func(context.Context, []byte) error) error {
	ch, err := c.channel(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := ch.Close(); closeErr != nil {
			c.log.Error("close rabbitmq channel", zap.Error(closeErr))
		}
	}()
	if err := ch.Qos(1, 0, false); err != nil {
		return fmt.Errorf("rabbitmq qos: %w", err)
	}
	deliveries, err := ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("consume %s: %w", queue, ctx.Err())
		case d, ok := <-deliveries:
			if !ok {
				return fmt.Errorf("queue %s closed", queue)
			}
			handleErr := handle(ctx, d.Body)
			if handleErr != nil && !errors.Is(handleErr, context.Canceled) {
				c.log.Error("drop rabbitmq message", zap.String("queue", queue), zap.Error(handleErr))
			}
			if err := settle(ctx, d, handleErr); err != nil {
				return fmt.Errorf("consume %s: %w", queue, err)
			}
		}
	}
}

func settle(ctx context.Context, d amqp.Delivery, handleErr error) error {
	if handleErr == nil {
		if err := d.Ack(false); err != nil {
			return fmt.Errorf("ack: %w", err)
		}
		return nil
	}
	requeue := errors.Is(handleErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled)
	if err := d.Nack(false, requeue); err != nil {
		return fmt.Errorf("nack: %w", err)
	}
	if requeue {
		return context.Canceled
	}
	return nil
}

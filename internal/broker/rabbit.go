package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Radiushina/avatar-service/internal/telemetry"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
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

func (c *Client) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("rabbitmq ping: %w", err)
	}
	if c == nil || c.conn == nil || c.conn.IsClosed() {
		return errors.New("rabbitmq connection closed")
	}
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq ping: %w", err)
	}
	if err := ch.Close(); err != nil {
		return fmt.Errorf("rabbitmq ping: %w", err)
	}
	return nil
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
	ctx, span := otel.Tracer("broker").Start(ctx, "publish "+routingKey)
	defer span.End()

	body, err := json.Marshal(event)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("encode event: %w", err)
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	headers := amqp.Table{}
	for key, value := range carrier {
		headers[key] = value
	}
	err = p.ch.PublishWithContext(ctx, exchange, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    messageID(event),
		Timestamp:    time.Now().UTC(),
		Headers:      headers,
		Body:         body,
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
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
			msgCtx := otel.GetTextMapPropagator().Extract(ctx, amqpCarrier(d.Headers))
			msgCtx, span := otel.Tracer("broker").Start(msgCtx, "consume "+queue)
			handleErr := handle(msgCtx, d.Body)
			if handleErr != nil {
				span.RecordError(handleErr)
				span.SetStatus(codes.Error, handleErr.Error())
			}
			span.End()
			if handleErr != nil && !errors.Is(handleErr, context.Canceled) {
				c.log.Error("drop rabbitmq message", zap.String("queue", queue), zap.Error(handleErr))
			}
			if err := settle(ctx, d, handleErr); err != nil {
				return fmt.Errorf("consume %s: %w", queue, err)
			}
		}
	}
}

func (c *Client) WatchQueueDepth(stop <-chan struct{}) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		c.sampleQueueDepth()
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func (c *Client) sampleQueueDepth() {
	if c == nil || c.conn == nil || c.conn.IsClosed() {
		return
	}
	for _, queue := range []string{KeyUploaded, KeyDeleted} {
		ch, err := c.conn.Channel()
		if err != nil {
			return
		}
		q, declErr := ch.QueueDeclarePassive(queue, true, false, false, false, nil)
		closeErr := ch.Close()
		if declErr != nil || closeErr != nil {
			continue
		}
		telemetry.QueueDepth.WithLabelValues(queue).Set(float64(q.Messages))
	}
}

func amqpCarrier(headers amqp.Table) propagation.MapCarrier {
	carrier := propagation.MapCarrier{}
	for key, value := range headers {
		text, ok := value.(string)
		if ok {
			carrier[key] = text
		}
	}
	return carrier
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

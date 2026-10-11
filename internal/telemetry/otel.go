package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func InitTracerProvider(ctx context.Context) (func(), error) {
	// Создаём gRPC Exporter (порт 4317)
	exporter, err := otlptracegrpc.New(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Метаинформация (Resource)
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}
	// Инициализируем TracerProvider
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		// Используем BatchSpanProcessor для эффективности (отправляет пачками)
		sdktrace.WithBatcher(exporter),
		// Sampler: AlwaysSample пишет 100% запросов (хорошо для дебага)
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	// Регистрируем глобальный провайдер
	otel.SetTracerProvider(tracerProvider)

	// Настраиваем propagation контекста.
	// Это позволяет передавать TraceID в заголовках запросов к другим сервисам.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Возвращаем функцию для корректного завершения (flush данных перед выходом).
	// Не используем ctx запуска: к моменту defer он уже отменён сигналом.
	return func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := tracerProvider.Shutdown(shutCtx); err != nil {
			otel.Handle(err)
		}
	}, nil
}

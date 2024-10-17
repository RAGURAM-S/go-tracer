package main

import (
	"context"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func newTraceExporter(ctx context.Context) (trace.SpanExporter, error) {
	return otlptracegrpc.New(ctx)
}

func newLogExporter(ctx context.Context) (log.Exporter, error) {
	return otlploggrpc.New(ctx)
}

func newTraceProvider(r *resource.Resource, exp sdktrace.SpanExporter) *sdktrace.TracerProvider {
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(r),
	)
}

func newLogProvider(r *resource.Resource, logExporter log.Exporter) *log.LoggerProvider {
	return log.NewLoggerProvider(
		log.WithResource(r),
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
}

var (
	Tracer oteltrace.Tracer
	Logger *slog.Logger
)

func main() {
	ctx := context.Background()

	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("test-service"),
		),
	)
	if err != nil {
		panic(err)
	}

	traceExporter, err := newTraceExporter(ctx)
	if err != nil {
		panic(err)
	}

	traceProvider := newTraceProvider(r, traceExporter)
	defer traceProvider.Shutdown(ctx)

	otel.SetTracerProvider(traceProvider)
	Tracer = traceProvider.Tracer("test-app-tracer")

	logExporter, err := newLogExporter(ctx)
	if err != nil {
		panic(err)
	}

	logProvider := newLogProvider(r, logExporter)
	defer logProvider.Shutdown(ctx)

	global.SetLoggerProvider(logProvider)

	Logger = otelslog.NewLogger("test-app-logger")

	app := fiber.New()

	app.Get("/", func(c *fiber.Ctx) error {
		ctx, span := Tracer.Start(c.Context(), "hello-world-span")
		defer span.End()

		Logger.Log(ctx, slog.LevelDebug, "hello world handler in action")

		return c.SendString("Hello, World!!!")
	})

	app.Listen(":9999")
}

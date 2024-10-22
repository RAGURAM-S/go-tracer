package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func newTraceExporter(ctx context.Context) (trace.SpanExporter, error) {
	return otlptracegrpc.New(ctx)
}

func newLogExporter(ctx context.Context) (log.Exporter, error) {
	return otlploggrpc.New(ctx)
}

func newMetricExporter(ctx context.Context) (metric.Exporter, error) {
	return otlpmetricgrpc.New(
		ctx,
		otlpmetricgrpc.WithCompressor("gzip"),
	)
}

func newTraceProvider(r *resource.Resource, exp trace.SpanExporter) *trace.TracerProvider {
	return trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(r),
	)
}

func newLogProvider(r *resource.Resource, logExporter log.Exporter) *log.LoggerProvider {
	return log.NewLoggerProvider(
		log.WithResource(r),
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
}

func newMeterProvider(r *resource.Resource, metricExporter metric.Exporter, metricFrequency time.Duration) *metric.MeterProvider {
	meterProvider := metric.NewMeterProvider(
		metric.WithResource(r),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			// Default is 1m
			metric.WithInterval(metricFrequency))),
	)
	return meterProvider
}

const (
	metricNameHttpServerDuration       = "http.server.duration"
	metricNameHttpServerRequestSize    = "http.server.request.size"
	metricNameHttpServerResponseSize   = "http.server.response.size"
	metricNameHttpServerActiveRequests = "http.server.active_requests"

	UnitDimensionless = "1"
	UnitBytes         = "By"
	UnitMilliseconds  = "ms"
)

var (
	Tracer oteltrace.Tracer
	Logger *slog.Logger
	Meter  otelmetric.Meter
)

type config struct {
	Next              func(*fiber.Ctx) bool
	TracerProvider    oteltrace.TracerProvider
	MeterProvider     otelmetric.MeterProvider
	Port              *int
	Propagators       propagation.TextMapPropagator
	ServerName        *string
	SpanNameFormatter func(*fiber.Ctx) string
}

// func metricsTrackingMiddleware(meter otelmetric.Meter) fiber.Handler {
// 	httpServerDuration, err := meter.Float64Histogram(metricNameHttpServerDuration, otelmetric.WithUnit(UnitMilliseconds), otelmetric.WithDescription("measures the duration inbound HTTP requests"))
// 	if err != nil {
// 		otel.Handle(err)
// 	}
// 	httpServerRequestSize, err := meter.Int64Histogram(metricNameHttpServerRequestSize, otelmetric.WithUnit(UnitBytes), otelmetric.WithDescription("measures the size of HTTP request messages"))
// 	if err != nil {
// 		otel.Handle(err)
// 	}
// 	httpServerResponseSize, err := meter.Int64Histogram(metricNameHttpServerResponseSize, otelmetric.WithUnit(UnitBytes), otelmetric.WithDescription("measures the size of HTTP response messages"))
// 	if err != nil {
// 		otel.Handle(err)
// 	}
// 	httpServerActiveRequests, err := meter.Int64UpDownCounter(metricNameHttpServerActiveRequests, otelmetric.WithUnit(UnitDimensionless), otelmetric.WithDescription("measures the number of concurrent HTTP requests that are currently in-flight"))
// 	if err != nil {
// 		otel.Handle(err)
// 	}
// 	return func(c *fiber.Ctx) error {
// 		defer func() {

// 		}()

// 		return nil
// 	}
// }

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

	// set up tracer
	traceExporter, err := newTraceExporter(ctx)
	if err != nil {
		panic(err)
	}

	traceProvider := newTraceProvider(r, traceExporter)
	defer traceProvider.Shutdown(ctx)

	otel.SetTracerProvider(traceProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	Tracer = traceProvider.Tracer("test-app-tracer")

	// set up log tracking
	logExporter, err := newLogExporter(ctx)
	if err != nil {
		panic(err)
	}

	logProvider := newLogProvider(r, logExporter)
	defer logProvider.Shutdown(ctx)

	global.SetLoggerProvider(logProvider)

	Logger = otelslog.NewLogger("test-app-logger")

	// set up metrics tracking
	metricExporter, err := newMetricExporter(ctx)
	if err != nil {
		panic(err)
	}

	meterProvider := newMeterProvider(r, metricExporter, 1*time.Minute)
	defer meterProvider.Shutdown(ctx)

	otel.SetMeterProvider(meterProvider)

	Meter = otel.Meter("test-app-metrics")

	app := fiber.New()
	// app.Use(metricsTrackingMiddleware)

	app.Get("/", func(c *fiber.Ctx) error {
		ctx, span := Tracer.Start(c.Context(), "hello-world-span")
		defer span.End()

		Logger.Log(ctx, slog.LevelDebug, "hello world handler in action")

		return c.SendString("Hello, World!!!")
	})

	app.Listen(":9999")
}

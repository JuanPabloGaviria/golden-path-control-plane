package observability

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/juanpablogaviria/golden-path-control-plane/internal/config"
)

type Metrics struct {
	Registry           *prometheus.Registry
	HTTPRequestsTotal  *prometheus.CounterVec
	HTTPRequestLatency *prometheus.HistogramVec
	WorkerJobsTotal    *prometheus.CounterVec
}

func NewLogger(level string) (*slog.Logger, error) {
	var slogLevel slog.Level
	if err := slogLevel.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
		return nil, fmt.Errorf("observability: parse log level: %w", err)
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})), nil
}

func NewMetrics(namespace string) (*Metrics, error) {
	registry := prometheus.NewRegistry()
	metrics := &Metrics{
		Registry: registry,
		HTTPRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests by route and status.",
		}, []string{"route", "method", "status"}),
		HTTPRequestLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request latency by route and method.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"route", "method"}),
		WorkerJobsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "worker_jobs_total",
			Help:      "Worker job outcomes by type and status.",
		}, []string{"type", "status"}),
	}

	if err := registry.Register(collectors.NewGoCollector()); err != nil {
		return nil, err
	}

	if err := registry.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
		return nil, err
	}

	if err := registry.Register(metrics.HTTPRequestsTotal); err != nil {
		return nil, err
	}

	if err := registry.Register(metrics.HTTPRequestLatency); err != nil {
		return nil, err
	}

	if err := registry.Register(metrics.WorkerJobsTotal); err != nil {
		return nil, err
	}

	return metrics, nil
}

func SetupTracing(ctx context.Context, cfg config.ObservabilityConfig) (func(context.Context) error, error) {
	resource, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create resource: %w", err)
	}

	if strings.TrimSpace(cfg.OTLPEndpoint) == "" {
		provider := sdktrace.NewTracerProvider(
			sdktrace.WithResource(resource),
		)
		otel.SetTracerProvider(provider)
		return provider.Shutdown, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(strings.TrimSpace(cfg.OTLPEndpoint)),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create otlp exporter: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(3*time.Second)),
		sdktrace.WithResource(resource),
	)

	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

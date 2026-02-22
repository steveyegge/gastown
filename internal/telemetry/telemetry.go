// Package telemetry initializes OpenTelemetry providers for metric and log export.
//
// Metrics → VictoriaMetrics via OTLP HTTP
// Logs    → VictoriaLogs via OTLP HTTP
//
// Enabled by setting at least one of:
//
//	GT_OTEL_METRICS_URL  (default: http://localhost:8428/opentelemetry/api/v1/push)
//	GT_OTEL_LOGS_URL     (default: http://localhost:9428/insert/opentelemetry/v1/logs)
//
// Telemetry is best-effort: initialization errors are returned but do not
// affect normal gt operation — callers should log and continue.
//
// Init is idempotent: multiple calls return the same provider.
package telemetry

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

const (
	// EnvMetricsURL is the env var for the VictoriaMetrics OTLP endpoint.
	EnvMetricsURL = "GT_OTEL_METRICS_URL"

	// EnvLogsURL is the env var for the VictoriaLogs OTLP endpoint.
	EnvLogsURL = "GT_OTEL_LOGS_URL"

	// DefaultMetricsURL is VictoriaMetrics' OTLP push endpoint.
	DefaultMetricsURL = "http://localhost:8428/opentelemetry/api/v1/push"

	// DefaultLogsURL is VictoriaLogs' OTLP insert endpoint.
	DefaultLogsURL = "http://localhost:9428/insert/opentelemetry/v1/logs"

	// ExportInterval is how often metrics are pushed to VictoriaMetrics.
	ExportInterval = 30 * time.Second
)

// package-level state for idempotent Init.
var (
	initMu         sync.Mutex
	initDone       bool
	globalProvider *Provider
)

// Provider wraps OTel SDK providers and their shutdown functions.
type Provider struct {
	shutdowns    []func(context.Context) error
	shutdownMu   sync.Mutex
	shutdownDone bool
}

// Shutdown flushes all pending data and stops the OTel providers.
// Idempotent: safe to call more than once.
// Should be called with a deadline context (e.g. 5s timeout) on process exit.
func (p *Provider) Shutdown(ctx context.Context) error {
	p.shutdownMu.Lock()
	defer p.shutdownMu.Unlock()
	if p.shutdownDone {
		return nil
	}
	p.shutdownDone = true

	var errs []error
	for _, fn := range p.shutdowns {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("telemetry shutdown errors: %v", errs)
	}
	return nil
}

// Init initializes OTel metric and log providers.
//
// Idempotent: subsequent calls (same or different arguments) return the
// provider created on the first call. The serviceName and serviceVersion
// passed to later calls are silently ignored — the first caller wins.
// In practice each GT process calls Init exactly once, so this is not an
// issue. If multiple packages call Init, ensure the entry-point (main or
// cobra root) calls it first with the correct service name.
//
// Returns (nil, nil) if neither GT_OTEL_METRICS_URL nor GT_OTEL_LOGS_URL is set,
// so that telemetry is strictly opt-in. Set either variable to activate.
//
// When active, defaults are used for any unset endpoint:
//
//	metrics → http://localhost:8428/opentelemetry/api/v1/push
//	logs    → http://localhost:9428/insert/opentelemetry/v1/logs
func Init(ctx context.Context, serviceName, serviceVersion string) (*Provider, error) {
	initMu.Lock()
	defer initMu.Unlock()
	if initDone {
		return globalProvider, nil
	}

	metricsURL := os.Getenv(EnvMetricsURL)
	logsURL := os.Getenv(EnvLogsURL)

	// Both unset → telemetry disabled, not an error.
	if metricsURL == "" && logsURL == "" {
		initDone = true
		globalProvider = nil
		return nil, nil
	}
	if metricsURL == "" {
		metricsURL = DefaultMetricsURL
	}
	if logsURL == "" {
		logsURL = DefaultLogsURL
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
		resource.WithHost(),
		resource.WithOS(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	p := &Provider{}

	// Metrics → VictoriaMetrics
	metricExp, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpointURL(metricsURL),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP metric exporter: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(metricExp,
				sdkmetric.WithInterval(ExportInterval),
			),
		),
	)
	otel.SetMeterProvider(mp)
	p.shutdowns = append(p.shutdowns, mp.Shutdown)
	initInstruments()

	// Logs → VictoriaLogs
	logExp, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpointURL(logsURL),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExp)),
	)
	global.SetLoggerProvider(lp)
	p.shutdowns = append(p.shutdowns, lp.Shutdown)

	initDone = true
	globalProvider = p
	return p, nil
}

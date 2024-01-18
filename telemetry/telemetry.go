package telemetry

import (
	"context"
	"errors"
	"github.com/agoda-com/opentelemetry-go/otelslog"
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"log/slog"
)

type Telemetry struct {
	tp *sdktrace.TracerProvider
	lp *sdklogs.LoggerProvider
}

func (t *Telemetry) TracerProvider() *sdktrace.TracerProvider {
	return t.tp
}

func (t *Telemetry) LoggerProvider() *sdklogs.LoggerProvider {
	return t.lp
}

func (t *Telemetry) ForceFlush(ctx context.Context) error {
	return flushAll(ctx, t.tp, t.lp)
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	return shutdownAll(ctx, t.tp, t.lp)
}

type config struct {
	resourceFn func(ctx context.Context, serviceName string) (*sdkresource.Resource, error)
	tpFn       func(ctx context.Context, resource *sdkresource.Resource) (*sdktrace.TracerProvider, error)
	lpFn       func(ctx context.Context, resource *sdkresource.Resource) (*sdklogs.LoggerProvider, error)
}

type Option func(c config) config

func WithResource(fn func(ctx context.Context, serviceName string) (*sdkresource.Resource, error)) Option {
	return func(c config) config {
		c.resourceFn = fn
		return c
	}
}

func WithTracerProvider(fn func(ctx context.Context, resource *sdkresource.Resource) (*sdktrace.TracerProvider, error)) Option {
	return func(c config) config {
		c.tpFn = fn
		return c
	}
}

func WithLoggerProvider(fn func(ctx context.Context, resource *sdkresource.Resource) (*sdklogs.LoggerProvider, error)) Option {
	return func(c config) config {
		c.lpFn = fn
		return c
	}
}

func NewTelemetry(ctx context.Context, serviceName string, options ...Option) (*Telemetry, error) {
	var cfg config
	for _, option := range options {
		cfg = option(cfg)
	}

	if cfg.resourceFn == nil || cfg.tpFn == nil || cfg.lpFn == nil {
		return nil, errors.New("missing configuration")
	}

	resource, err := cfg.resourceFn(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	tp, err := cfg.tpFn(ctx, resource)
	if err != nil {
		return nil, err
	}

	lp, err := cfg.lpFn(ctx, resource)
	if err != nil {
		if e := tp.Shutdown(ctx); e != nil {
			err = errors.Join(err, e)
		}

		return nil, err
	}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(xray.Propagator{})

	log := slog.New(otelslog.NewOtelHandler(lp, &otelslog.HandlerOptions{}))
	slog.SetDefault(log)

	return &Telemetry{
		tp: tp,
		lp: lp,
	}, nil
}

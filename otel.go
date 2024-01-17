package main

import (
	"context"
	"errors"
	"github.com/agoda-com/opentelemetry-go/otelslog"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs/otlplogsgrpc"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/stdout/stdoutlogs"
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"go.opentelemetry.io/contrib/detectors/aws/lambda"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"io"
	"log/slog"
	"os"
)

type provider interface {
	Shutdown(ctx context.Context) error
}

func WithFunctionURLTracing(ctx context.Context, serviceName string, fn func(ctx context.Context, tp *sdktrace.TracerProvider, lp *sdklogs.LoggerProvider) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	resource, err := newFunctionURLResource(ctx, serviceName)
	if err != nil {
		return err
	}

	tp, err := newFunctionURLTracing(ctx, resource)
	if err != nil {
		return err
	}

	lp, err := newFunctionURLLogging(ctx, resource)
	if err != nil {
		if tpShutdownErr := tp.Shutdown(ctx); tpShutdownErr != nil {
			err = errors.Join(err, tpShutdownErr)
		}

		return err
	}

	return shutdownAll(ctx, fn(ctx, tp, lp), tp, lp)
}

func newFunctionURLTracing(ctx context.Context, resource *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	tp, err := newFunctionURLTracerProvider(ctx, resource)
	if err != nil {
		return nil, err
	}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(xray.Propagator{})

	return tp, nil
}

func newFunctionURLTracerProvider(ctx context.Context, resource *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	// see: xrayconfig.NewTracerProvider(ctx)
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exp),
		sdktrace.WithIDGenerator(xray.NewIDGenerator()),
		sdktrace.WithResource(resource),
	), nil
}

func WithLocalTracing(ctx context.Context, fn func(ctx context.Context, tp *sdktrace.TracerProvider, lp *sdklogs.LoggerProvider) error) error {
	tpExp, err := stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(io.Discard))
	if err != nil {
		return err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(tpExp),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	lpExp, err := stdoutlogs.NewExporter(
		stdoutlogs.WithWriter(os.Stdout),
	)

	lp := sdklogs.NewLoggerProvider(
		sdklogs.WithBatcher(lpExp),
	)

	otelLog := slog.New(otelslog.NewOtelHandler(lp, &otelslog.HandlerOptions{}))
	slog.SetDefault(otelLog)

	return shutdownAll(ctx, fn(ctx, tp, lp), tp, lp)
}

func newFunctionURLLogging(ctx context.Context, resource *sdkresource.Resource) (*sdklogs.LoggerProvider, error) {
	lp, err := newFunctionURLLoggerProvider(ctx, resource)
	if err != nil {
		return nil, err
	}

	otelLog := slog.New(otelslog.NewOtelHandler(lp, &otelslog.HandlerOptions{}))
	slog.SetDefault(otelLog)

	return lp, nil
}

func newFunctionURLLoggerProvider(ctx context.Context, resource *sdkresource.Resource) (*sdklogs.LoggerProvider, error) {
	client := otlplogsgrpc.NewClient(
		otlplogsgrpc.WithInsecure(),
	)

	exp, err := otlplogs.NewExporter(ctx, otlplogs.WithClient(client))
	if err != nil {
		return nil, err
	}

	lp := sdklogs.NewLoggerProvider(
		sdklogs.WithBatcher(exp),
		sdklogs.WithResource(resource),
	)

	return lp, nil
}

func newFunctionURLResource(ctx context.Context, serviceName string) (*sdkresource.Resource, error) {
	detector := lambda.NewResourceDetector()
	resource, err := detector.Detect(ctx)
	if err != nil {
		return nil, err
	}

	attrs := append(resource.Attributes(), semconv.ServiceName(serviceName))
	return sdkresource.NewWithAttributes(resource.SchemaURL(), attrs...), nil
}

func shutdownAll(ctx context.Context, err error, providers ...provider) error {
	for _, p := range providers {
		if e := p.Shutdown(ctx); e != nil {
			err = errors.Join(err, e)
		}
	}

	return err
}

package main

import (
	"context"
	"errors"
	"fmt"
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
)

func WithFunctionURLTracing(ctx context.Context, serviceName string, fn func(ctx context.Context, tp *sdktrace.TracerProvider) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	tp, err := newFunctionURLTracing(ctx, serviceName)
	if err != nil {
		return err
	}

	err = fn(ctx, tp)
	if shutdownErr := tp.Shutdown(ctx); shutdownErr != nil {
		err = errors.Join(err, fmt.Errorf("error shutting down TracerProvider: %w", shutdownErr))
	}

	return err
}

func newFunctionURLTracing(ctx context.Context, serviceName string) (*sdktrace.TracerProvider, error) {
	tp, err := newFunctionURLTracerProvider(ctx, serviceName)
	if err != nil {
		return nil, err
	}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(xray.Propagator{})

	return tp, nil
}

func newFunctionURLTracerProvider(ctx context.Context, serviceName string) (*sdktrace.TracerProvider, error) {
	// see: xrayconfig.NewTracerProvider(ctx)
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}

	detector := lambda.NewResourceDetector()
	resource, err := detector.Detect(ctx)
	if err != nil {
		return nil, err
	}

	attrs := append(resource.Attributes(), semconv.ServiceName(serviceName))
	resource = sdkresource.NewWithAttributes(resource.SchemaURL(), attrs...)

	return sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exp),
		sdktrace.WithIDGenerator(xray.NewIDGenerator()),
		sdktrace.WithResource(resource),
	), nil
}

func WithLocalTracing(ctx context.Context, fn func(ctx context.Context, tp *sdktrace.TracerProvider) error) error {
	// writer := os.Stdout
	writer := io.Discard
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(writer))
	if err != nil {
		return err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exp),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	return fn(ctx, tp)
}

package telemetry

import (
	"context"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"io"
)

func NewLambdaTracerProvider(ctx context.Context, resource *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
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

func NewLocalTracerProvider(ctx context.Context, resource *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint(), stdouttrace.WithWriter(io.Discard))
	if err != nil {
		return nil, err
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exp),
	), nil
}

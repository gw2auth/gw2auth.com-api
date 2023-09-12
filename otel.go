package main

import (
	"context"
	"go.opentelemetry.io/contrib/detectors/aws/lambda"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.19.0"
)

func NewFunctionURLTracing(serviceName string, ctx context.Context) (*sdktrace.TracerProvider, propagation.TextMapPropagator, error) {
	tp, err := NewFunctionURLTracerProvider(serviceName, ctx)
	if err != nil {
		return nil, nil, err
	}

	// prop := NewFunctionURLPropagator()
	prop := xray.Propagator{}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(prop)

	return tp, prop, nil
}

func NewFunctionURLTracerProvider(serviceName string, ctx context.Context) (*sdktrace.TracerProvider, error) {
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

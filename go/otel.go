package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
	"go.opentelemetry.io/contrib/detectors/aws/lambda"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.19.0"
	"go.opentelemetry.io/otel/trace"
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
		sdktrace.WithBatcher(exp),
		// sdktrace.WithIDGenerator(NewFunctionURLIDGenerator()),
		sdktrace.WithIDGenerator(xray.NewIDGenerator()),
		sdktrace.WithResource(resource),
	), nil
}

func NewFunctionURLPropagator() propagation.TextMapPropagator {
	return functionURLPropagator{
		xrayProp: xray.Propagator{},
	}
}

type functionURLPropagator struct {
	xrayProp xray.Propagator
}

func (p functionURLPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	p.xrayProp.Inject(ctx, carrier)
}

func (p functionURLPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return ctx
}

func (p functionURLPropagator) Fields() []string {
	return p.xrayProp.Fields()
}

func NewFunctionURLIDGenerator() sdktrace.IDGenerator {
	return &functionURLIDGenerator{
		fallbackIDGen: xray.NewIDGenerator(),
		xrayProp:      xray.Propagator{},
	}
}

type functionURLIDGenerator struct {
	fallbackIDGen sdktrace.IDGenerator
	xrayProp      xray.Propagator
}

func (gen *functionURLIDGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	tid, sid := gen.fallbackIDGen.NewIDs(ctx)

	if event, ok := handler.GetSourceEvent(ctx).(events.LambdaFunctionURLRequest); ok {
		if val, ok := event.Headers["x-amzn-trace-id"]; ok && val != "" {
			ctx := gen.xrayProp.Extract(ctx, propagation.MapCarrier{"X-Amzn-Trace-Id": val})
			span := trace.SpanContextFromContext(ctx)

			if span.HasTraceID() {
				tid = span.TraceID()
			}

			if span.HasSpanID() {
				sid = span.SpanID()
			}
		}
	}

	fmt.Printf("NewIDs: trace=%v span=%v\n", tid, sid)

	return tid, sid
}

func (gen *functionURLIDGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	return gen.fallbackIDGen.NewSpanID(ctx, traceID)
}

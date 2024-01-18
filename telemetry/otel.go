package telemetry

import (
	"context"
	"errors"
	"go.opentelemetry.io/contrib/detectors/aws/lambda"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

type provider interface {
	ForceFlush(ctx context.Context) error
	Shutdown(ctx context.Context) error
}

func flushAll(ctx context.Context, providers ...provider) error {
	var err error
	for _, p := range providers {
		if p == nil {
			continue
		}

		if e := p.ForceFlush(ctx); e != nil {
			err = errors.Join(err, e)
		}
	}

	return err
}

func shutdownAll(ctx context.Context, providers ...provider) error {
	var err error
	for _, p := range providers {
		if p == nil {
			continue
		}

		if e := p.Shutdown(ctx); e != nil {
			err = errors.Join(err, e)
		}
	}

	return err
}

func NewLambdaResource(ctx context.Context, serviceName string) (*sdkresource.Resource, error) {
	return sdkresource.New(
		ctx,
		sdkresource.WithDetectors(lambda.NewResourceDetector()),
		sdkresource.WithAttributes(semconv.ServiceName(serviceName)),
	)
}

func NewLocalResource(ctx context.Context, serviceName string) (*sdkresource.Resource, error) {
	return sdkresource.New(
		ctx,
		sdkresource.WithAttributes(semconv.ServiceName(serviceName)),
	)
}

package telemetry

import (
	"context"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/otlp/otlplogs/otlplogsgrpc"
	"github.com/agoda-com/opentelemetry-logs-go/exporters/stdout/stdoutlogs"
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	"log/slog"
	"os"
)

func NewLambdaLoggerProvider(ctx context.Context, resource *sdkresource.Resource) (*sdklogs.LoggerProvider, error) {
	client := otlplogsgrpc.NewClient(otlplogsgrpc.WithInsecure())
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

func NewLocalLoggerProvider(ctx context.Context, resource *sdkresource.Resource) (*sdklogs.LoggerProvider, error) {
	exp, err := stdoutlogs.NewExporter(stdoutlogs.WithWriter(os.Stdout))
	if err != nil {
		return nil, err
	}

	lp := sdklogs.NewLoggerProvider(
		sdklogs.WithBatcher(exp),
		sdklogs.WithResource(resource),
	)

	return lp, nil
}

func Error(err error) slog.Attr {
	return slog.String("err", err.Error())
}

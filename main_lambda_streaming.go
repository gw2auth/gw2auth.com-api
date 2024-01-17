//go:build lambda && lambda.norpc

package main

import (
	"context"
	"encoding/json"
	sdklogs "github.com/agoda-com/opentelemetry-logs-go/sdk/logs"
	"github.com/aws/aws-lambda-go/lambda"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/its-felix/aws-lambda-go-http-adapter/adapter"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"os"
)

func main() {
	ctx := context.Background()
	err := WithFunctionURLTracing(ctx, "GW2AuthAPILambda", func(ctx context.Context, tp *sdktrace.TracerProvider, lp *sdklogs.LoggerProvider) error {
		return WithEchoServer(ctx, func(ctx context.Context, app *echo.Echo) error {
			h := handler.NewFunctionURLStreamingHandler(adapter.NewEchoAdapter(app))
			lambda.Start(otellambda.InstrumentHandler(h, otellambda.WithTracerProvider(tp), otellambda.WithFlusher(tp)))

			return nil
		}, WithFlusher(tp), WithFlusher(lp))
	})

	if err != nil {
		panic(err)
	}
}

func loadSecrets(ctx context.Context) (Secrets, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return Secrets{}, err
	}

	bucket := os.Getenv("SECRETS_BUCKET")
	key := os.Getenv("SECRETS_KEY")

	otelaws.AppendMiddlewares(&cfg.APIOptions)
	s3Client := s3.NewFromConfig(cfg)

	res, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil {
		return Secrets{}, err
	}

	defer res.Body.Close()
	var secrets Secrets
	if err = json.NewDecoder(res.Body).Decode(&secrets); err != nil {
		return Secrets{}, err
	}

	return secrets, nil
}

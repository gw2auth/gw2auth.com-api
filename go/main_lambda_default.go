//go:build lambda && !lambda.norpc

package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/its-felix/aws-lambda-go-http-adapter/adapter"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda/xrayconfig"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
)

func main() {
	ctx := context.Background()
	tp, err := xrayconfig.NewTracerProvider(ctx)
	if err != nil {
		fmt.Printf("error creating tracer provider: %v", err)
	}

	defer func(ctx context.Context) {
		var err error
		if err = tp.Shutdown(ctx); err != nil {
			fmt.Printf("error shutting down tracer provider: %v", err)
		}
	}(ctx)

	prop := xray.Propagator{}

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(prop)

	app := newEchoServer(tp, prop)
	h := handler.NewFunctionURLHandler(adapter.NewEchoAdapter(app))
	lambda.Start(h)
}

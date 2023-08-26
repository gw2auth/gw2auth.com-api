//go:build lambda && lambda.norpc

package main

import (
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/its-felix/aws-lambda-go-http-adapter/adapter"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
)

func main() {
	tp, _, err := NewFunctionURLTracing("GW2AuthAPILambda", context.Background())
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			fmt.Printf("error shutting down tracer provider: %v", err)
		}
	}()

	app, shutdownFunc, err := newConfiguredEchoServer()
	defer shutdownFunc()
	if err != nil {
		panic(err)
	}

	h := handler.NewFunctionURLStreamingHandler(adapter.NewEchoAdapter(app))
	lambda.Start(otellambda.InstrumentHandler(h))
}

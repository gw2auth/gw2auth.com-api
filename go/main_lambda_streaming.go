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
	ctx := context.Background()
	tp, _, err := NewFunctionURLTracing("GW2AuthAPILambda", ctx)
	if err != nil {
		panic(err)
	}

	defer func(ctx context.Context) {
		var err error
		if err = tp.Shutdown(ctx); err != nil {
			fmt.Printf("error shutting down tracer provider: %v", err)
		}
	}(ctx)

	app := newEchoServer()
	h := handler.NewFunctionURLStreamingHandler(adapter.NewEchoAdapter(app))

	lambda.Start(otellambda.InstrumentHandler(h))
}

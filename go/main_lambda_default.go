//go:build lambda && !lambda.norpc

package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/its-felix/aws-lambda-go-http-adapter/adapter"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
)

func main() {
	mux := newServer()
	h := handler.NewFunctionURLHandler(adapter.NewVanillaAdapter(mux))
	lambda.Start(h)
}

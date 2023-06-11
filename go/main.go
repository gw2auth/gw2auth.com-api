//go:build !lambda

package main

import (
	"go.opentelemetry.io/otel"
)

func main() {
	tp := otel.GetTracerProvider()
	prop := otel.GetTextMapPropagator()

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(prop)

	app := newEchoServer(tp, prop)
	_ = app.Start(":8080")
}

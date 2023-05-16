package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/its-felix/aws-lambda-go-http-adapter/adapter/vanilla"
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
	"net/http"
	"time"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	mux.HandleFunc("/api/v2/ping-delayed", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)

		for i := 0; i < 10; i++ {
			_, _ = w.Write([]byte("pong"))
			time.Sleep(1 * time.Second)
		}
	})

	adapter := vanilla.NewAdapter(mux)
	h := handler.NewFunctionURLHandler(adapter)

	lambda.Start(h)
}

package main

import (
	"net/http"
	"time"
)

func newServer() *http.ServeMux {
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

	return mux
}

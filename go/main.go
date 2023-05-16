//go:build !lambda

package main

import "net/http"

func main() {
	mux := newServer()
	_ = http.ListenAndServe(":8080", mux)
}

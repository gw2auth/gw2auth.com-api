//go:build !lambda

package main

func main() {
	app := newEchoServer()
	_ = app.Start(":8080")
}

//go:build !lambda

package main

func main() {
	app, shutdownFunc, err := newConfiguredEchoServer()
	defer shutdownFunc()
	if err != nil {
		panic(err)
	}

	_ = app.Start(":8080")
}

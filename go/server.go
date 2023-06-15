package main

import (
	"github.com/its-felix/aws-lambda-go-http-adapter/handler"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"time"
)

func newLogger() *otelzap.Logger {
	rootLog, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	return otelzap.New(rootLog, otelzap.WithMinLevel(zap.InfoLevel))
}

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

func newEchoServer() *echo.Echo {
	rootLog := newLogger()
	httpClient := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	app := echo.New()
	app.GET("/api/v2/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, "pong")
	})

	app.GET("/api/v2/ping-delayed", func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderContentType, echo.MIMETextPlain)
		c.Response().WriteHeader(http.StatusOK)

		for i := 0; i < 100; i++ {
			_, _ = c.Response().Write([]byte(strconv.Itoa(i)))
			_, _ = c.Response().Write([]byte("\n"))

			time.Sleep(time.Millisecond * 10)
		}

		return nil
	})

	app.GET("/otel", func(c echo.Context) error {
		ctx := c.Request().Context()
		log := rootLog.Ctx(ctx)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/open-telemetry/opentelemetry-go/releases/latest", nil)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		res, err := httpClient.Do(req)
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}
		err = res.Body.Close()
		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		log.Info("invocation finished, returning event")
		return c.JSONPretty(http.StatusOK, handler.GetSourceEvent(ctx), "\t")
	})

	mw := otelecho.Middleware("api.gw2auth.com")
	app.Use(mw)

	return app
}

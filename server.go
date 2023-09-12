package main

import (
	"context"
	crdbpgx "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgxv5"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"
	"net/http"
	"os"
	"strconv"
	"time"
)

func newLogger() (*otelzap.Logger, error) {
	rootLog, err := zap.NewProduction()
	if err != nil {
		return nil, err
	}

	return otelzap.New(rootLog, otelzap.WithMinLevel(zap.InfoLevel)), nil
}

func newPgx() (*pgx.Conn, error) {
	config, err := pgx.ParseConfig(os.Getenv("DATABASE_URL"))
	if err != nil {
		return nil, err
	}

	config.Tracer = otelpgx.NewTracer()
	config.RuntimeParams["application_name"] = "api.gw2auth.com"

	return pgx.ConnectConfig(context.Background(), config)
}

func newHttpClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

func newEchoServer(log *otelzap.Logger, httpClient *http.Client, conn *pgx.Conn) *echo.Echo {
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

	app.GET("/api/v2/application/summary", func(c echo.Context) error {
		ctx := c.Request().Context()
		log := log.Ctx(ctx)

		var numAccounts uint32
		var numApiTokens uint32
		var numAccVerifications uint32
		var numApps uint32
		var numAppClients uint32
		var numAppClientAccs uint32

		err := crdbpgx.ExecuteTx(ctx, conn, pgx.TxOptions{}, func(tx pgx.Tx) error {
			sql := `SELECT
	(SELECT COUNT(*) FROM accounts),
    (SELECT COUNT(*) FROM gw2_account_api_tokens WHERE last_valid_time = last_valid_check_time),
    (SELECT COUNT(*) FROM gw2_account_verifications),
    (SELECT COUNT(*) FROM applications),
    (SELECT COUNT(*) FROM application_clients),
    (SELECT COUNT(*) FROM application_client_accounts WHERE ARRAY_LENGTH(authorized_scopes, 1) > 0)`

			return tx.QueryRow(ctx, sql).Scan(
				&numAccounts,
				&numApiTokens,
				&numAccVerifications,
				&numApps,
				&numAppClients,
				&numAppClientAccs,
			)
		})

		if err != nil {
			return c.String(http.StatusInternalServerError, err.Error())
		}

		log.Info("successfully processed application summary request")

		return c.JSONPretty(http.StatusOK, map[string]uint32{
			"accounts":                  numAccounts,
			"gw2ApiTokens":              numApiTokens,
			"verifiedGw2Accounts":       numAccVerifications,
			"applications":              numApps,
			"applicationClients":        numAppClients,
			"applicationClientAccounts": numAppClientAccs,
		}, "\t")
	})

	mw := otelecho.Middleware("api.gw2auth.com")
	app.Use(mw)

	return app
}

func newConfiguredEchoServer() (*echo.Echo, func(), error) {
	log, err := newLogger()
	if err != nil {
		return nil, shutdownFunc(nil, nil, nil), err
	}

	conn, err := newPgx()
	if err != nil {
		log.Error("failed to retrieve pgx conn", zap.Error(err))
		return nil, shutdownFunc(log, nil, nil), err
	}

	app := newEchoServer(log, newHttpClient(), conn)
	return app, shutdownFunc(log, conn, app), nil
}

func shutdownFunc(log *otelzap.Logger, conn *pgx.Conn, app *echo.Echo) func() {
	return func() {
		if log != nil {
			defer log.Sync()
		}

		if conn != nil {
			defer conn.Close(context.Background())
		}

		if app != nil {
			defer app.Close()
		}
	}
}

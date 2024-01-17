package web

import (
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

func newEchoWithMiddleware(pool *pgxpool.Pool, conv *service.SessionJwtConverter) *echo.Echo {
	e := echo.New()
	e.Use(
		Middleware(pool),
		DeleteHistoricalCookiesMiddleware(),
		AuthenticatedMiddleware(conv),
	)

	return e
}

package web

import (
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/uptrace/opentelemetry-go-extra/otelzap"
	"go.uber.org/zap"
)

func newEchoWithMiddleware(pool *pgxpool.Pool, conv *service.SessionJwtConverter) *echo.Echo {
	e := echo.New()
	e.Use(
		Middleware(otelzap.New(zap.NewNop()), pool),
		AuthenticatedMiddleware(conv),
	)

	return e
}

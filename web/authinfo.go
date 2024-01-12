package web

import (
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/labstack/echo/v4"
	"net/http"
	"time"
)

func AuthInfoEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		return c.JSON(http.StatusOK, map[string]string{
			"sessionId":           session.Id,
			"sessionCreationTime": session.CreationTime.Format(time.RFC3339),
			"issuer":              session.Issuer,
			"idAtIssuer":          session.IdAtIssuer,
		})
	})
}

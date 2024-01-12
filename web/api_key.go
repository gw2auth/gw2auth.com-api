package web

import (
	"errors"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
	"slices"
)

type modifyRedirectURIsRequest struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

func ModifyDevApplicationClientRedirectURIsEndpoint() echo.HandlerFunc {
	return wrapApiKeyAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, apiKey auth.ApiKey) error {
		var err error
		var body modifyRedirectURIsRequest
		if err = c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		var clientId uuid.UUID
		if clientId, err = uuid.FromString(c.Param("client_id")); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		body.Add = preprocessRedirectURIs(apiKey.ApplicationId, clientId, body.Add)
		if len(body.Add) > 50 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("at most 50 redirect URIs may be added"))
		}

		if err = validateRedirectURIs(body.Add); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		var prevRedirectURIs, newRedirectURIs []string
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
UPDATE application_clients
SET redirect_uris = prep.new_redirect_uris
FROM (
	SELECT
		app_clients.id AS id,
	    app_clients.redirect_uris AS prev_redirect_uris,
	    COALESCE(
			(
				SELECT ARRAY_AGG(DISTINCT sub.uri)
				FROM (
					SELECT UNNEST(ARRAY_CAT(app_clients.redirect_uris, $4::TEXT[])) AS uri
					EXCEPT ALL
					SELECT UNNEST($5::TEXT[]) AS uri
				) sub
			),
	    	app_clients.redirect_uris
	    ) AS new_redirect_uris
    FROM application_clients app_clients
    INNER JOIN applications apps
    ON app_clients.application_id = apps.id
    WHERE apps.account_id = $1
    AND apps.id = $2
    AND app_clients.id = $3
) prep
WHERE application_clients.id = prep.id
RETURNING
    prep.prev_redirect_uris,
    application_clients.redirect_uris
`

			return tx.QueryRow(ctx, sql, apiKey.AccountId, apiKey.ApplicationId, clientId, body.Add, body.Remove).Scan(
				&prevRedirectURIs,
				&newRedirectURIs,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if slices.Equal(prevRedirectURIs, newRedirectURIs) {
			return c.NoContent(http.StatusNotModified)
		}

		return c.NoContent(http.StatusNoContent)
	})
}

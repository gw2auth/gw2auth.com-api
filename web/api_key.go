package web

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

type modifyRedirectURIsRequest struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

type applicationUserDeletion struct {
	Id           uuid.UUID `json:"id"`
	DeletionTime time.Time `json:"deletion_time"`
}

type applicationUsersResponse struct {
	Watermark time.Time                 `json:"watermark"`
	Users     []applicationUserDeletion `json:"users"`
}

func ApplicationDeletedUsersEndpoint() echo.HandlerFunc {
	return wrapApiKeyAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, apiKey auth.ApiKey) error {
		since := time.Date(1969, time.January, 1, 0, 0, 0, 0, time.UTC)
		if sinceRaw := c.QueryParam("since"); sinceRaw != "" {
			var err error
			if since, err = time.Parse(time.RFC3339, sinceRaw); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err)
			}
		}

		watermark := time.Now().UTC().Add(-time.Minute)
		users := make([]applicationUserDeletion, 0)
		ctx := c.Request().Context()
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    account_subs.account_sub,
    account_registry.deletion_time
FROM account_registry
INNER JOIN application_account_subs account_subs
ON account_registry.id = account_subs.account_id
WHERE account_subs.application_id = $1
AND account_registry.deletion_time < $2
AND account_registry.deletion_time >= $3
ORDER BY account_registry.deletion_time, account_subs.account_sub
`
			rows, err := tx.Query(ctx, sql, apiKey.ApplicationId, watermark, since)
			if err != nil {
				return err
			}

			users, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (applicationUserDeletion, error) {
				var user applicationUserDeletion
				return user, row.Scan(&user.Id, &user.DeletionTime)
			})
			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, applicationUsersResponse{
			Watermark: watermark,
			Users:     users,
		})
	})
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
		slog.InfoContext(
			ctx,
			"modifying application client redirect URIs via api key",
			slog.Any("client.redirect_uri.add", body.Add),
			slog.Any("client.redirect_uri.remove", body.Remove),
		)

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

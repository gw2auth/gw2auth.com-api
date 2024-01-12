package web

import (
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
	"time"
)

type applicationForList struct {
	Id               uuid.UUID  `json:"id"`
	DisplayName      string     `json:"displayName"`
	UserId           uuid.UUID  `json:"userId"`
	LastUsed         *time.Time `json:"lastUsed,omitempty"`
	AuthorizedScopes []string   `json:"authorizedScopes"`
}

type application struct {
	DisplayName           string                  `json:"displayName"`
	UserId                uuid.UUID               `json:"userId"`
	LastUsed              *time.Time              `json:"lastUsed,omitempty"`
	AuthorizedScopes      []string                `json:"authorizedScopes"`
	AuthorizedGw2Accounts []applicationGw2Account `json:"authorizedGw2Accounts"`
}

type applicationGw2Account struct {
	Id          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName"`
}

func UserApplicationsEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()

		var results []applicationForList
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    apps.id,
    MAX(apps.display_name),
    MAX(app_acc_sub.account_sub),
    MAX(app_client_auth.last_update_time),
    COALESCE(
        ARRAY_AGG(DISTINCT app_client_auth.scope) FILTER ( WHERE app_client_auth.scope IS NOT NULL ),
        ARRAY[]::TEXT[]
    )
FROM application_accounts app_accs
INNER JOIN application_account_subs app_acc_sub
USING (application_id, account_id)
INNER JOIN applications apps
ON app_accs.application_id = apps.id
LEFT JOIN (
    SELECT
        app_client_accs.application_id,
        app_client_accs.account_id,
        auth.last_update_time,
        UNNEST(auth.authorized_scopes) AS scope
    FROM application_client_authorizations auth
    INNER JOIN application_client_accounts app_client_accs
    USING (application_client_id, account_id)
    WHERE auth.refresh_token_expires_at > NOW()
    AND ARRAY_LENGTH(auth.authorized_scopes, 1) > 0
) AS app_client_auth
ON app_client_auth.application_id = app_accs.application_id AND app_client_auth.account_id = app_accs.account_id
WHERE app_accs.account_id = $1
GROUP BY apps.id
`
			rows, err := tx.Query(ctx, sql, session.AccountId)
			if err != nil {
				return err
			}

			results, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (applicationForList, error) {
				var app applicationForList
				return app, row.Scan(
					&app.Id,
					&app.DisplayName,
					&app.UserId,
					&app.LastUsed,
					&app.AuthorizedScopes,
				)
			})

			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, results)
	})
}

func UserApplicationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		applicationId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		var result application
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    MAX(apps.display_name),
    MAX(app_acc_sub.account_sub),
    MAX(app_client_auth.last_update_time),
    COALESCE(
        ARRAY_AGG(DISTINCT app_client_auth.scope) FILTER ( WHERE app_client_auth.scope IS NOT NULL ),
        ARRAY[]::TEXT[]
    ),
    COALESCE(
    	ARRAY_AGG(DISTINCT JSONB_BUILD_OBJECT(
    		'id', app_client_auth.gw2_account_id,
    		'name', app_client_auth.gw2_account_name,
    		'displayName', app_client_auth.gw2_acc_display_name
		)) FILTER ( WHERE app_client_auth.gw2_account_id IS NOT NULL ),
		ARRAY[]::JSONB[]
    )
FROM application_accounts app_accs
INNER JOIN application_account_subs app_acc_sub
USING (application_id, account_id)
INNER JOIN applications apps
ON app_accs.application_id = apps.id
LEFT JOIN (
    SELECT
        app_client_accs.application_id AS application_id,
        app_client_accs.account_id AS account_id,
        auth.last_update_time AS last_update_time,
        UNNEST(auth.authorized_scopes) AS scope,
        gw2_acc.gw2_account_id AS gw2_account_id,
        gw2_acc.gw2_account_name AS gw2_account_name,
        gw2_acc.display_name AS gw2_acc_display_name
    FROM application_client_authorizations auth
    INNER JOIN application_client_accounts app_client_accs
    USING (application_client_id, account_id)
    LEFT JOIN application_client_authorization_gw2_accounts app_client_auth_gw2_acc
    ON auth.id = app_client_auth_gw2_acc.application_client_authorization_id
    LEFT JOIN gw2_accounts gw2_acc
    ON app_client_auth_gw2_acc.account_id = gw2_acc.account_id AND app_client_auth_gw2_acc.gw2_account_id = gw2_acc.gw2_account_id
    WHERE auth.refresh_token_expires_at > NOW()
    AND ARRAY_LENGTH(auth.authorized_scopes, 1) > 0
) AS app_client_auth
ON app_client_auth.application_id = app_accs.application_id AND app_client_auth.account_id = app_accs.account_id
WHERE app_accs.account_id = $1
AND app_accs.application_id = $2
`
			return tx.QueryRow(ctx, sql, session.AccountId, applicationId).Scan(
				&result.DisplayName,
				&result.UserId,
				&result.LastUsed,
				&result.AuthorizedScopes,
				&result.AuthorizedGw2Accounts,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, result)
	})
}

func DeleteUserApplicationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		applicationId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM application_accounts
WHERE account_id = $1
AND application_id = $2
`
			_, err := tx.Exec(ctx, sql, session.AccountId, applicationId)
			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

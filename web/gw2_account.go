package web

import (
	"errors"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"log/slog"
	"net/http"
	"time"
)

type verificationStatus string

const (
	verificationStatusNone     verificationStatus = "NONE"
	verificationStatusPending  verificationStatus = "PENDING"
	verificationStatusVerified verificationStatus = "VERIFIED"
)

type gw2AccountForList struct {
	Id                 uuid.UUID          `json:"id"`
	Name               string             `json:"name"`
	DisplayName        string             `json:"displayName"`
	CreationTime       time.Time          `json:"creationTime"`
	VerificationStatus verificationStatus `json:"verificationStatus"`
	ApiToken           *string            `json:"apiToken,omitempty"`
	AuthorizedApps     uint32             `json:"authorizedApps"`
}

type gw2Account struct {
	Name               string             `json:"name"`
	DisplayName        string             `json:"displayName"`
	CreationTime       time.Time          `json:"creationTime"`
	VerificationStatus verificationStatus `json:"verificationStatus"`
	ApiToken           *apiToken          `json:"apiToken,omitempty"`
	AuthorizedApps     []authorizedApp    `json:"authorizedApps"`
}

type apiToken struct {
	Value        string           `json:"value"`
	CreationTime time.Time        `json:"creationTime"`
	Permissions  []gw2.Permission `json:"permissions"`
}

type authorizedApp struct {
	Id       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	LastUsed time.Time `json:"lastUsed"`
}

type gw2AccountUpdate struct {
	DisplayName string `json:"displayName,omitempty"`
}

func Gw2AccountsEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()

		var results []gw2AccountForList
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    gw2_acc.gw2_account_id,
    MAX(gw2_acc.gw2_account_name),
    MAX(gw2_acc.display_name),
    MAX(gw2_acc.creation_time),
    COUNT(gw2_acc_ver.account_id) > 0,
    COUNT(gw2_acc_ver_pend.account_id) > 0,
    MAX(gw2_acc_tk.gw2_api_token),
    COUNT(DISTINCT app_client.application_id) FILTER ( WHERE app_client_auth.refresh_token_expires_at > NOW() AND ARRAY_LENGTH(app_client_auth.authorized_scopes, 1) > 0 )
FROM gw2_accounts gw2_acc
LEFT JOIN gw2_account_verifications gw2_acc_ver
	USING (account_id, gw2_account_id)
LEFT JOIN gw2_account_verification_pending_challenges gw2_acc_ver_pend
	USING (account_id, gw2_account_id)
LEFT JOIN gw2_account_api_tokens gw2_acc_tk
	USING (account_id, gw2_account_id)
LEFT JOIN application_client_authorization_gw2_accounts app_client_auth_gw2_acc
	USING (account_id, gw2_account_id)
LEFT JOIN application_client_authorizations app_client_auth
	ON app_client_auth_gw2_acc.application_client_authorization_id = app_client_auth.id
LEFT JOIN application_clients app_client
	ON app_client_auth.application_client_id = app_client.id
WHERE gw2_acc.account_id = $1
GROUP BY gw2_acc.gw2_account_id
`
			rows, err := tx.Query(ctx, sql, session.AccountId)
			if err != nil {
				return err
			}

			results, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (gw2AccountForList, error) {
				var acc gw2AccountForList
				var verified bool
				var pendingVer bool

				err := row.Scan(
					&acc.Id,
					&acc.Name,
					&acc.DisplayName,
					&acc.CreationTime,
					&verified,
					&pendingVer,
					&acc.ApiToken,
					&acc.AuthorizedApps,
				)
				if err != nil {
					return acc, err
				}

				if verified {
					acc.VerificationStatus = verificationStatusVerified
				} else if pendingVer {
					acc.VerificationStatus = verificationStatusPending
				} else {
					acc.VerificationStatus = verificationStatusNone
				}

				return acc, nil
			})

			return err
		})
		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, results)
	})
}

func Gw2AccountEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		gw2AccountId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()

		var acc gw2Account
		var verified bool
		var pendingVer bool
		var apiTokenRaw *struct {
			Value        string    `json:"value"`
			CreationTime time.Time `json:"creationTime"`
			Permissions  int32     `json:"permissions"`
			Exists       bool      `json:"exists"`
		}
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    MAX(gw2_acc.gw2_account_name),
    MAX(gw2_acc.display_name),
    MAX(gw2_acc.creation_time),
    COUNT(gw2_acc_ver.account_id) > 0,
    COUNT(gw2_acc_ver_pend.account_id) > 0,
    MAX(JSONB_BUILD_OBJECT(
		'value', gw2_acc_tk.gw2_api_token,
		'creationTime', gw2_acc_tk.creation_time,
        'permissions', gw2_acc_tk.gw2_api_permissions_bit_set
	)) FILTER ( WHERE gw2_acc_tk.gw2_account_id IS NOT NULL ),
    COALESCE(ARRAY_AGG(JSONB_BUILD_OBJECT(
    	'id', authorized_apps.app_id,
		'name', authorized_apps.display_name,
		'lastUsed', authorized_apps.last_used
	)) FILTER ( WHERE authorized_apps.app_id IS NOT NULL ), ARRAY[]::JSONB[])
FROM gw2_accounts gw2_acc
LEFT JOIN gw2_account_verifications gw2_acc_ver
	USING (account_id, gw2_account_id)
LEFT JOIN gw2_account_verification_pending_challenges gw2_acc_ver_pend
	USING (account_id, gw2_account_id)
LEFT JOIN gw2_account_api_tokens gw2_acc_tk
	USING (account_id, gw2_account_id)
LEFT JOIN (
    SELECT
        app_client_auth_gw2_acc.account_id AS account_id,
        app_client_auth_gw2_acc.gw2_account_id AS gw2_account_id,
        app.id AS app_id,
        MAX(app.display_name) AS display_name,
        MAX(app_client_auth.last_update_time) AS last_used
    FROM application_client_authorization_gw2_accounts app_client_auth_gw2_acc
    INNER JOIN application_client_authorizations app_client_auth
        ON app_client_auth_gw2_acc.application_client_authorization_id = app_client_auth.id
	INNER JOIN application_clients app_client
		ON app_client_auth.application_client_id = app_client.id
	INNER JOIN applications app
		ON app_client.application_id = app.id
    WHERE app_client_auth.refresh_token_expires_at > NOW()
    AND ARRAY_LENGTH(app_client_auth.authorized_scopes, 1) > 0
	GROUP BY app_client_auth_gw2_acc.account_id, app_client_auth_gw2_acc.gw2_account_id, app.id
) AS authorized_apps
	USING (account_id, gw2_account_id)
WHERE gw2_acc.account_id = $1 AND gw2_acc.gw2_account_id = $2
GROUP BY gw2_acc.gw2_account_id
`
			return tx.QueryRow(ctx, sql, session.AccountId, gw2AccountId).Scan(
				&acc.Name,
				&acc.DisplayName,
				&acc.CreationTime,
				&verified,
				&pendingVer,
				&apiTokenRaw,
				&acc.AuthorizedApps,
			)
		})
		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if verified {
			acc.VerificationStatus = verificationStatusVerified
		} else if pendingVer {
			acc.VerificationStatus = verificationStatusPending
		} else {
			acc.VerificationStatus = verificationStatusNone
		}

		if apiTokenRaw != nil {
			acc.ApiToken = &apiToken{
				Value:        apiTokenRaw.Value,
				CreationTime: apiTokenRaw.CreationTime,
				Permissions:  gw2.PermissionsFromBitSet(apiTokenRaw.Permissions),
			}
		}

		return c.JSON(http.StatusOK, acc)
	})
}

func UpdateGw2AccountEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		gw2AccountId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		var update gw2AccountUpdate
		if err = c.Bind(&update); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if update.DisplayName == "" || len(update.DisplayName) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("displayname must be between 1 and 100 characters"))
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"updating gw2account",
			slog.String("accountId", session.AccountId.String()),
			slog.String("gw2AccountId", gw2AccountId.String()),
			slog.String("displayName", update.DisplayName),
		)

		var rowsAffected int64
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
UPDATE gw2_accounts
SET display_name = $3
WHERE account_id = $1 AND gw2_account_id = $2
`
			tag, err := tx.Exec(ctx, sql, session.AccountId, gw2AccountId, update.DisplayName)
			if err != nil {
				return err
			}

			rowsAffected = tag.RowsAffected()
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if rowsAffected < 1 {
			return echo.NewHTTPError(http.StatusNotFound, map[string]string{})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

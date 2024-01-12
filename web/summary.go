package web

import (
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
)

type appSummary struct {
	Accounts         uint32 `json:"accounts"`
	ApiTokens        uint32 `json:"gw2ApiTokens"`
	AccVerifications uint32 `json:"verifiedGw2Accounts"`
	Apps             uint32 `json:"applications"`
	AppClients       uint32 `json:"applicationClients"`
	AppClientAccs    uint32 `json:"applicationClientAccounts"`
}

func AppSummaryEndpoint() echo.HandlerFunc {
	return wrapHandlerFunc(func(c echo.Context, rctx RequestContext) error {
		var res appSummary

		ctx := c.Request().Context()
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			sql := `
SELECT
	(SELECT COUNT(*) FROM accounts),
    (SELECT COUNT(*) FROM gw2_account_api_tokens WHERE last_valid_time = last_valid_check_time),
    (SELECT COUNT(*) FROM gw2_account_verifications),
    (SELECT COUNT(*) FROM applications),
    (SELECT COUNT(*) FROM application_clients),
    (SELECT COUNT(*) FROM application_client_accounts WHERE ARRAY_LENGTH(authorized_scopes, 1) > 0)
`
			return tx.QueryRow(ctx, sql).Scan(
				&res.Accounts,
				&res.ApiTokens,
				&res.AccVerifications,
				&res.Apps,
				&res.AppClients,
				&res.AppClientAccs,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, res)
	})
}

package web

import (
	"errors"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/its-felix/shine"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
	"slices"
	"time"
)

type apiTokenAddOrUpdate struct {
	ApiToken string `json:"apiToken,omitempty"`
}

func AddOrUpdateApiTokenEndpoint(gw2ApiClient *gw2.ApiClient) echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var expectGw2AccountId uuid.UUID
		if idRaw := c.Param("id"); idRaw != "" {
			if id, err := uuid.FromString(idRaw); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err)
			} else {
				expectGw2AccountId = id
			}
		}

		expectAdd := c.Request().Method == http.MethodPut

		var body apiTokenAddOrUpdate
		if err := c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if body.ApiToken == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "the apitoken is invalid")
		}

		chAccount := make(chan shine.Result[gw2.Account], 1)
		chTokenInfo := make(chan shine.Result[gw2.TokenInfo], 1)

		ctx := c.Request().Context()
		go func() {
			defer close(chAccount)
			chAccount <- shine.NewResult(gw2ApiClient.Account(ctx, body.ApiToken))
		}()

		go func() {
			defer close(chTokenInfo)
			chTokenInfo <- shine.NewResult(gw2ApiClient.TokenInfo(ctx, body.ApiToken))
		}()

		resAccount, resTokenInfo := <-chAccount, <-chTokenInfo
		if resAccount.IsErr() {
			return resAccount.Err().Map(httpErrorForGw2ApiError).Unwrap()
		}

		if resTokenInfo.IsErr() {
			return resTokenInfo.Err().Map(httpErrorForGw2ApiError).Unwrap()
		}

		gw2Acc, tokenInfo := resAccount.Unwrap(), resTokenInfo.Unwrap()
		if !expectGw2AccountId.IsNil() && expectGw2AccountId != gw2Acc.Id {
			return echo.NewHTTPError(http.StatusBadRequest, "the provided apitoken does not belong to the expected gw2account")
		} else if !slices.Contains(tokenInfo.Permissions, gw2.PermissionAccount) {
			return echo.NewHTTPError(http.StatusBadRequest, "the provided apitoken does not provide the account permission")
		}

		gw2ApiPermissionsBitSet := gw2.PermissionsToBitSet(tokenInfo.Permissions)

		creationTime := time.Now()
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			sql := `
SELECT
	(
	    SELECT gw2_api_permissions_bit_set
	    FROM gw2_account_api_tokens
	    WHERE account_id = $1
		AND gw2_account_id = $2
	),
    (
        SELECT account_id
		FROM gw2_account_verifications
		WHERE gw2_account_id = $2
    )
`
			var existingTokenPermissionsBitSet *int32
			var verifiedForAccountId *uuid.UUID
			if err := tx.QueryRow(ctx, sql, session.AccountId, gw2Acc.Id).Scan(&existingTokenPermissionsBitSet, &verifiedForAccountId); err != nil {
				return err
			}

			if existingTokenPermissionsBitSet != nil && expectAdd && gw2ApiPermissionsBitSet&*existingTokenPermissionsBitSet != *existingTokenPermissionsBitSet {
				return echo.NewHTTPError(http.StatusBadRequest, "expected to add a new apitoken but an apitoken for this gw2account already exists with different permissions")
			}

			if verifiedForAccountId != nil && *verifiedForAccountId != session.AccountId {
				return echo.NewHTTPError(http.StatusNotAcceptable, "the gw2account is already verified for another gw2auth account")
			}

			sql = `
WITH gw2_account AS (
	INSERT INTO gw2_accounts
	(account_id, gw2_account_id, display_name, order_rank, gw2_account_name, creation_time, last_name_check_time)
	VALUES
	($1, $2, $3, $4, $5, $6, $6)
	ON CONFLICT (account_id, gw2_account_id) DO UPDATE SET
	gw2_account_name = EXCLUDED.gw2_account_name,
	last_name_check_time = EXCLUDED.last_name_check_time
	RETURNING *
)
INSERT INTO gw2_account_api_tokens
(account_id, gw2_account_id, gw2_api_token, gw2_api_permissions_bit_set, creation_time, last_valid_time, last_valid_check_time)
SELECT
    gw2_account.account_id,
    gw2_account.gw2_account_id,
    $7,
    $8,
    $6,
    $6,
    $6
FROM gw2_account
ON CONFLICT (account_id, gw2_account_id)
DO UPDATE SET
gw2_api_token = EXCLUDED.gw2_api_token,
gw2_api_permissions_bit_set = EXCLUDED.gw2_api_permissions_bit_set,
creation_time = EXCLUDED.creation_time,
last_valid_time = EXCLUDED.last_valid_time,
last_valid_check_time = EXCLUDED.last_valid_check_time
`
			_, err := tx.Exec(
				ctx,
				sql,
				session.AccountId,
				gw2Acc.Id,
				gw2Acc.Name,
				"A",
				gw2Acc.Name,
				creationTime,
				body.ApiToken,
				gw2ApiPermissionsBitSet,
			)

			return err
		})

		if err != nil {
			var httpError *echo.HTTPError
			if errors.As(err, &httpError) {
				return httpError
			} else {
				return util.NewEchoPgxHTTPError(err)
			}
		}

		return c.JSON(http.StatusOK, apiToken{
			Value:        body.ApiToken,
			CreationTime: creationTime,
			Permissions:  tokenInfo.Permissions,
		})
	})
}

func DeleteApiTokenEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		gw2AccountId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM gw2_account_api_tokens
WHERE account_id = $1
AND gw2_account_id = $2
`
			_, err := tx.Exec(ctx, sql, session.AccountId, gw2AccountId)
			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

func httpErrorForGw2ApiError(err error) error {
	if errors.Is(err, gw2.ErrInvalidApiToken) {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	} else if gw2.IsApiError(err) {
		return echo.NewHTTPError(http.StatusBadGateway, err)
	}

	return echo.NewHTTPError(http.StatusInternalServerError, err)
}

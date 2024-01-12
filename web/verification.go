package web

import (
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
	"time"
)

type verificationActiveChallenge struct {
	ChallengeId          int                   `json:"challengeId"`
	State                string                `json:"state"`
	CreationTime         time.Time             `json:"creationTime"`
	AvailableGw2Accounts []availableGw2Account `json:"availableGw2Accounts"`
}

type verificationPendingChallenge struct {
	Gw2AccountId          uuid.UUID `json:"gw2AccountId"`
	Gw2AccountName        string    `json:"gw2AccountName"`
	Gw2AccountDisplayName string    `json:"gw2AccountDisplayName"`
	ChallengeId           int       `json:"challengeId"`
	State                 string    `json:"state"`
	CreationTime          time.Time `json:"creationTime"`
	SubmitTime            time.Time `json:"submitTime"`
	TimeoutTime           time.Time `json:"timeoutTime"`
}

type availableGw2Account struct {
	Id                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	DisplayName       string    `json:"displayName"`
	ApiToken          string    `json:"apiToken"`
	PermissionsBitSet *int32    `json:"permissionsBitSet,omitempty"`
}

func VerificationActiveEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()
		var result verificationActiveChallenge
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    ch.challenge_id,
    ch.state,
    ch.creation_time,
    (
        SELECT COALESCE(
            ARRAY_AGG(JSON_BUILD_OBJECT(
                'id', gw2_acc.gw2_account_id,
                'name', gw2_acc.gw2_account_name,
                'displayName', gw2_acc.display_name,
                'apiToken', gw2_acc_tk.gw2_api_token,
                'permissionsBitSet', gw2_acc_tk.gw2_api_permissions_bit_set
            )) FILTER ( WHERE gw2_acc.gw2_account_id IS NOT NULL ),
            ARRAY[]::JSONB[]
        )
        FROM gw2_accounts gw2_acc
        INNER JOIN gw2_account_api_tokens gw2_acc_tk
        USING (account_id, gw2_account_id)
        LEFT JOIN gw2_account_verifications gw2_acc_ver
        USING (account_id, gw2_account_id)
        LEFT JOIN gw2_account_verification_pending_challenges gw2_acc_pending_ver
        USING (account_id, gw2_account_id)
        WHERE gw2_acc.account_id = ch.account_id
        AND gw2_acc_ver.account_id IS NULL
        AND gw2_acc_pending_ver.account_id IS NULL
    )
FROM gw2_account_verification_challenges ch
WHERE ch.account_id = $1
`
			return tx.QueryRow(ctx, sql, session.AccountId).Scan(
				&result.ChallengeId,
				&result.State,
				&result.CreationTime,
				&result.AvailableGw2Accounts,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		var reqPerms util.Set[gw2.Permission]
		switch result.ChallengeId {
		case 1:
			reqPerms = util.NewSet(gw2.PermissionAccount)

		case 2:
			reqPerms = util.NewSet(gw2.PermissionAccount, gw2.PermissionTradingpost)

		case 3:
			reqPerms = util.NewSet(gw2.PermissionAccount, gw2.PermissionCharacters)
		}

		resultingAccs := make([]availableGw2Account, 0, len(result.AvailableGw2Accounts))
		for _, acc := range result.AvailableGw2Accounts {
			perms := util.NewSet(gw2.PermissionsFromBitSet(*acc.PermissionsBitSet)...)
			if perms.ContainsAll(reqPerms) {
				acc.PermissionsBitSet = nil
				resultingAccs = append(resultingAccs, acc)
			}
		}

		result.AvailableGw2Accounts = resultingAccs

		return c.JSON(http.StatusOK, result)
	})
}

func VerificationPendingEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()
		var result []verificationPendingChallenge
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    ch.gw2_account_id,
    gw2_acc.gw2_account_name,
    gw2_acc.display_name,
    ch.challenge_id,
    ch.state,
    ch.creation_time,
    ch.submit_time,
    ch.timeout_time
FROM gw2_account_verification_pending_challenges ch
INNER JOIN gw2_accounts gw2_acc
USING (account_id, gw2_account_id)
WHERE ch.account_id = $1
`
			rows, err := tx.Query(ctx, sql, session.AccountId)
			if err != nil {
				return err
			}

			result, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (verificationPendingChallenge, error) {
				var p verificationPendingChallenge
				return p, row.Scan(
					&p.Gw2AccountId,
					&p.Gw2AccountName,
					&p.Gw2AccountDisplayName,
					&p.ChallengeId,
					&p.State,
					&p.CreationTime,
					&p.SubmitTime,
					&p.TimeoutTime,
				)
			})

			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, result)
	})
}

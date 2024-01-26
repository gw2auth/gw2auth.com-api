package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
)

type apiTokenAddOrUpdateResponse struct {
	Value        string           `json:"value"`
	CreationTime time.Time        `json:"creationTime"`
	Permissions  []gw2.Permission `json:"permissions"`
	Verified     bool             `json:"verified"`
}

type apiTokenAddOrUpdateRequest struct {
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

		var body apiTokenAddOrUpdateRequest
		if err := c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if body.ApiToken == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "the apitoken is invalid")
		}

		var gw2Acc gw2.Account
		var tokenInfo gw2.TokenInfo

		ctx := c.Request().Context()
		gw2Acc, tokenInfo, err := accountAndTokenInfo(ctx, gw2ApiClient, body.ApiToken)
		if err != nil {
			return httpErrorForGw2ApiError(err)
		}

		if !expectGw2AccountId.IsNil() && expectGw2AccountId != gw2Acc.Id {
			return echo.NewHTTPError(http.StatusBadRequest, "the provided apitoken does not belong to the expected gw2account")
		} else if !slices.Contains(tokenInfo.Permissions, gw2.PermissionAccount) {
			return echo.NewHTTPError(http.StatusBadRequest, "the provided apitoken does not provide the account permission")
		}

		isVerifiedAdd := expectAdd && verifyTokenName(tokenInfo.Name, session.Id)
		gw2ApiPermissionsBitSet := gw2.PermissionsToBitSet(tokenInfo.Permissions)

		slog.InfoContext(
			ctx,
			"api token is being added or updated",
			slog.String("gw2account.id", gw2Acc.Id.String()),
			slog.String("gw2account.name", gw2Acc.Name),
			slog.String("gw2account.token.name", tokenInfo.Name),
			slog.Any("gw2account.token.permissions", tokenInfo.Permissions),
			slog.Bool("is_verified_add", isVerifiedAdd),
		)

		creationTime := time.Now()
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
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

			if !isVerifiedAdd && verifiedForAccountId != nil && *verifiedForAccountId != session.AccountId {
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

			if err != nil {
				return err
			}

			if isVerifiedAdd {
				sqls := []string{
					"DELETE FROM gw2_account_api_tokens WHERE gw2_account_id = $1 AND account_id != $2",
					"INSERT INTO gw2_account_verifications (gw2_account_id, account_id) VALUES ($1, $2) ON CONFLICT (gw2_account_id) DO UPDATE SET account_id = EXCLUDED.account_id",
				}

				for _, sql = range sqls {
					_, err = tx.Exec(
						ctx,
						sql,
						gw2Acc.Id,
						session.AccountId,
					)

					if err != nil {
						return err
					}
				}
			}

			return nil
		})

		if err != nil {
			var httpError *echo.HTTPError
			if errors.As(err, &httpError) {
				return httpError
			} else {
				return util.NewEchoPgxHTTPError(err)
			}
		}

		return c.JSON(http.StatusOK, apiTokenAddOrUpdateResponse{
			Value:        body.ApiToken,
			CreationTime: creationTime,
			Permissions:  tokenInfo.Permissions,
			Verified:     isVerifiedAdd,
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
		slog.InfoContext(
			ctx,
			"deleting api token",
			slog.String("gw2account.id", gw2AccountId.String()),
		)

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

func ApiTokenVerificationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		name := verificationTokenNameForSession(session.Id)
		slog.InfoContext(c.Request().Context(), "generated new verification token name", slog.String("verification_token_name", name))

		return c.JSON(http.StatusOK, map[string]string{
			"tokenName": name,
		})
	})
}

func accountAndTokenInfo(ctx context.Context, client *gw2.ApiClient, token string) (gw2.Account, gw2.TokenInfo, error) {
	var gw2Acc gw2.Account
	var tokenInfo gw2.TokenInfo

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		var err error
		gw2Acc, err = client.Account(ctx, token)
		return err
	})

	group.Go(func() error {
		var err error
		tokenInfo, err = client.TokenInfo(ctx, token)
		return err
	})

	return gw2Acc, tokenInfo, group.Wait()
}

func httpErrorForGw2ApiError(err error) error {
	if errors.Is(err, gw2.ErrInvalidApiToken) {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	} else if gw2.IsApiError(err) {
		return echo.NewHTTPError(http.StatusBadGateway, err)
	}

	return echo.NewHTTPError(http.StatusInternalServerError, err)
}

func verifyTokenName(name, sessionId string) bool {
	prefix, suffix, ok := strings.Cut(strings.TrimRightFunc(name, unicode.IsSpace), "-")
	if !ok || prefix != "GW2Auth" {
		return false
	}

	b, err := base64.RawURLEncoding.DecodeString(suffix)
	if err != nil || len(b) != 20 {
		return false
	}

	buf := bytes.NewBuffer(b)

	// this check can be removed 6hrs after being deployed to prod
	var ts time.Time
	if timeSinceEpoch := time.Duration(binary.BigEndian.Uint32(buf.Next(4))); timeSinceEpoch >= 400000 && timeSinceEpoch < 500000 {
		ts = util.UnixZero().Add(timeSinceEpoch * time.Hour)
	} else {
		ts = util.UnixZero().Add(timeSinceEpoch * time.Minute)
	}

	now := time.Now()
	if now.Before(ts) || now.Sub(ts) >= (time.Hour*6) {
		return false
	}

	hash := sha256.Sum256([]byte(sessionId))
	return bytes.Equal(hash[:16], buf.Next(16))
}

func verificationTokenNameForSession(sessionId string) string {
	// 4 bytes for time, 16 bytes for hash
	b := make([]byte, 0, 20)

	minutesSinceEpoch := uint32(time.Since(util.UnixZero()) / time.Minute)
	b = binary.BigEndian.AppendUint32(b, minutesSinceEpoch)

	hash := sha256.Sum256([]byte(sessionId))
	b = append(b, hash[:16]...)

	return "GW2Auth-" + base64.RawURLEncoding.EncodeToString(b)
}

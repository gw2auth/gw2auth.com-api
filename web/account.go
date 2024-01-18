package web

import (
	"errors"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"log/slog"
	"net/http"
	"time"
)

type account struct {
	Federations []accountFederation        `json:"federations"`
	Sessions    []accountFederationSession `json:"sessions"`
}

type accountFederation struct {
	Issuer     string `json:"issuer"`
	IdAtIssuer string `json:"idAtIssuer"`
}

type accountFederationSession struct {
	Id           string    `json:"id"`
	Issuer       string    `json:"issuer"`
	IdAtIssuer   string    `json:"idAtIssuer"`
	CreationTime time.Time `json:"creationTime"`
}

func AccountEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()
		var result account
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
	(
		SELECT COALESCE(
			ARRAY_AGG(JSONB_BUILD_OBJECT(
				'issuer', issuer,
			    'idAtIssuer', id_at_issuer
			)) FILTER ( WHERE account_id IS NOT NULL ),
		    ARRAY[]::JSONB[]
		)
		FROM account_federations
		WHERE account_id = $1
	),
	(
	    SELECT COALESCE(
	    	ARRAY_AGG(JSONB_BUILD_OBJECT(
	    		'id', acc_fed_sess.id,
			    'issuer', acc_fed.issuer,
			    'idAtIssuer', acc_fed.id_at_issuer,
	    	    'creationTime', acc_fed_sess.creation_time
			)) FILTER ( WHERE account_id IS NOT NULL ),
		    ARRAY[]::JSONB[]
		)
	    FROM account_federation_sessions acc_fed_sess
	    INNER JOIN account_federations acc_fed
	    USING (issuer, id_at_issuer)
	    WHERE acc_fed.account_id = $1
	)
`
			return tx.QueryRow(ctx, sql, session.AccountId).Scan(
				&result.Federations,
				&result.Sessions,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, result)
	})
}

func DeleteAccountEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()
		var found bool
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = "DELETE FROM accounts WHERE id = $1"
			tags, err := tx.Exec(ctx, sql, session.AccountId)
			if err != nil {
				return err
			}

			found = tags.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !found {
			return echo.NewHTTPError(http.StatusNotFound)
		}

		return c.NoContent(http.StatusOK)
	})
}

func DeleteAccountFederationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		issuer, idAtIssuer := c.QueryParam("issuer"), c.QueryParam("idAtIssuer")
		if issuer == session.Issuer && idAtIssuer == session.IdAtIssuer {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("can not delete current federation"))
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"deleting account federation",
			slog.String("federation.issuer", issuer),
			slog.String("federation.id_at_issuer", idAtIssuer),
		)

		var found bool
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM account_federations
WHERE account_id = $1
AND issuer = $2
AND id_at_issuer = $3
`

			tags, err := tx.Exec(ctx, sql, session.AccountId, issuer, idAtIssuer)
			if err != nil {
				return err
			}

			found = tags.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !found {
			return echo.NewHTTPError(http.StatusNotFound)
		}

		return c.NoContent(http.StatusOK)
	})
}

func DeleteAccountFederationSessionEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		sessionId := c.QueryParam("id")
		if sessionId == session.Id {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("can not delete current session"))
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"deleting account federation session",
			slog.String("session.id", sessionId),
		)

		var found bool
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM account_federation_sessions
WHERE id = (
    SELECT acc_fed_sess.id
    FROM account_federation_sessions acc_fed_sess
    INNER JOIN account_federations acc_fed
    USING (issuer, id_at_issuer)
    WHERE acc_fed.account_id = $1
    AND acc_fed_sess.id = $2
)
`

			tags, err := tx.Exec(ctx, sql, session.AccountId, sessionId)
			if err != nil {
				return err
			}

			found = tags.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !found {
			return echo.NewHTTPError(http.StatusNotFound)
		}

		return c.NoContent(http.StatusOK)
	})
}

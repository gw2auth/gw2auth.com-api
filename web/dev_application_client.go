package web

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type devApplicationClientCreate struct {
	DisplayName      string   `json:"displayName"`
	RedirectURIs     []string `json:"redirectURIs"`
	RequiresApproval bool     `json:"requiresApproval"`
}

type devApplicationClient struct {
	CreationTime     time.Time `json:"creationTime"`
	DisplayName      string    `json:"displayName"`
	ApiVersion       uint32    `json:"apiVersion"`
	Type             string    `json:"type"`
	RedirectURIs     []string  `json:"redirectURIs"`
	RequiresApproval bool      `json:"requiresApproval"`
}

type devApplicationClientCreateResponse struct {
	Id               uuid.UUID `json:"id"`
	CreationTime     time.Time `json:"creationTime"`
	DisplayName      string    `json:"displayName"`
	ApiVersion       uint32    `json:"apiVersion"`
	Type             string    `json:"type"`
	RedirectURIs     []string  `json:"redirectURIs"`
	RequiresApproval bool      `json:"requiresApproval"`
	ClientSecret     string    `json:"clientSecret"`
}

type devAppClientUserUpdate struct {
	ApprovalStatus  string `json:"approvalStatus"`
	ApprovalMessage string `json:"approvalMessage"`
}

func CreateDevApplicationClientEndpoint() echo.HandlerFunc {
	const apiVersion = 0
	const clientType = "CONFIDENTIAL"

	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var err error
		var applicationId uuid.UUID
		if applicationId, err = uuid.FromString(c.Param("id")); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		var body devApplicationClientCreate
		if err = c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if body.DisplayName == "" || len(body.DisplayName) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("displayname must be between 1 and 100 characters"))
		}

		applicationClientId, err := uuid.NewV4()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		body.RedirectURIs = preprocessRedirectURIs(applicationId, applicationClientId, body.RedirectURIs)
		if len(body.RedirectURIs) < 1 || len(body.RedirectURIs) > 50 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("at least one and at most 50 redirect URIs might be added"))
		}

		if err = validateRedirectURIs(body.RedirectURIs); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		creationTime := time.Now()
		clientSecret, err := generateClientSecret()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		encodedClientSecret, err := encodeClientSecret(clientSecret)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"creating new application client",
			slog.String("application.id", applicationId.String()),
			slog.String("application.client.id", applicationClientId.String()),
			slog.String("application.client.name", body.DisplayName),
		)

		var created bool
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
INSERT INTO application_clients
(id, application_id, creation_time, display_name, client_secret, authorization_grant_types, redirect_uris, requires_approval, api_version, type)
SELECT
    $3,
    id,
    $4,
    $5,
    $6,
    $7,
    $8,
    $9,
    $10,
    $11
FROM applications
WHERE account_id = $1
AND id = $2
`
			tag, err := tx.Exec(
				ctx,
				sql,
				session.AccountId,
				applicationId,
				applicationClientId,
				creationTime,
				body.DisplayName,
				encodedClientSecret,
				[]string{"authorization_code", "refresh_token"},
				body.RedirectURIs,
				body.RequiresApproval,
				apiVersion,
				clientType,
			)
			if err != nil {
				return err
			}

			created = tag.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !created {
			return echo.NewHTTPError(http.StatusNotFound, errors.New("the application does not exist"))
		}

		return c.JSON(http.StatusOK, devApplicationClientCreateResponse{
			Id:               applicationClientId,
			CreationTime:     creationTime,
			DisplayName:      body.DisplayName,
			ApiVersion:       apiVersion,
			Type:             clientType,
			RedirectURIs:     body.RedirectURIs,
			RequiresApproval: body.RequiresApproval,
			ClientSecret:     clientSecret,
		})
	})
}

func DevApplicationClientEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var applicationId, clientId uuid.UUID
		if values, err := util.EchoAllParams(c, uuid.FromString, "app_id", "client_id"); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		} else {
			applicationId, clientId = values[0], values[1]
		}

		ctx := c.Request().Context()
		var result devApplicationClient
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    app_clients.creation_time,
    app_clients.display_name,
    app_clients.api_version,
    app_clients.type,
    app_clients.redirect_uris,
    app_clients.requires_approval
FROM application_clients app_clients
INNER JOIN applications apps
ON app_clients.application_id = apps.id
WHERE apps.account_id = $1
AND apps.id = $2
AND app_clients.id = $3
`
			return tx.QueryRow(ctx, sql, session.AccountId, applicationId, clientId).Scan(
				&result.CreationTime,
				&result.DisplayName,
				&result.ApiVersion,
				&result.Type,
				&result.RedirectURIs,
				&result.RequiresApproval,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, result)
	})
}

func RegenerateDevApplicationClientSecretEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var applicationId, clientId uuid.UUID
		if values, err := util.EchoAllParams(c, uuid.FromString, "app_id", "client_id"); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		} else {
			applicationId, clientId = values[0], values[1]
		}

		clientSecret, err := generateClientSecret()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		encodedClientSecret, err := encodeClientSecret(clientSecret)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"regenerating client secret for client",
			slog.String("application.id", applicationId.String()),
			slog.String("application.client.id", clientId.String()),
		)

		var updated bool
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
UPDATE application_clients
SET client_secret = $4
WHERE id = (
    SELECT app_clients.id
    FROM application_clients app_clients
    INNER JOIN applications apps
    ON app_clients.application_id = apps.id
    WHERE apps.account_id = $1
    AND apps.id = $2
    AND app_clients.id = $3
)
`

			tag, err := tx.Exec(ctx, sql, session.AccountId, applicationId, clientId, encodedClientSecret)
			if err != nil {
				return err
			}

			updated = tag.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !updated {
			return echo.NewHTTPError(http.StatusNotFound, errors.New("no rows were updated"))
		}

		return c.JSON(http.StatusOK, map[string]string{
			"clientSecret": clientSecret,
		})
	})
}

func UpdateDevApplicationClientRedirectURIsEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var applicationId, clientId uuid.UUID
		if values, err := util.EchoAllParams(c, uuid.FromString, "app_id", "client_id"); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		} else {
			applicationId, clientId = values[0], values[1]
		}

		var err error
		var redirectURIs []string
		if err = c.Bind(&redirectURIs); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		redirectURIs = preprocessRedirectURIs(applicationId, clientId, redirectURIs)
		if len(redirectURIs) < 1 || len(redirectURIs) > 50 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("at least one and at most 50 redirect URIs might be added"))
		}

		if err = validateRedirectURIs(redirectURIs); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"updating redirect uris for client",
			slog.String("application.id", applicationId.String()),
			slog.String("application.client.id", clientId.String()),
			slog.Any("application.client.redirect_uris", redirectURIs),
		)

		var prevRedirectURIs, newRedirectURIs []string
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
UPDATE application_clients
SET redirect_uris = $4
FROM (
	SELECT
		app_clients.id AS id,
	    app_clients.redirect_uris AS prev_redirect_uris
    FROM application_clients app_clients
    INNER JOIN applications apps
    ON app_clients.application_id = apps.id
    WHERE apps.account_id = $1
    AND apps.id = $2
    AND app_clients.id = $3
) prep
WHERE application_clients.id = prep.id
RETURNING prep.prev_redirect_uris, application_clients.redirect_uris
`

			return tx.QueryRow(ctx, sql, session.AccountId, applicationId, clientId, redirectURIs).Scan(
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

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

func DeleteDevApplicationClientEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var applicationId, clientId uuid.UUID
		if values, err := util.EchoAllParams(c, uuid.FromString, "app_id", "client_id"); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		} else {
			applicationId, clientId = values[0], values[1]
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"deleting application client",
			slog.String("application.id", applicationId.String()),
			slog.String("application.client.id", clientId.String()),
		)

		var deleted bool
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM application_clients
WHERE id = (
    SELECT app_clients.id
    FROM application_clients app_clients
    INNER JOIN applications apps
    ON app_clients.application_id = apps.id
    WHERE apps.account_id = $1
    AND apps.id = $2
    AND app_clients.id = $3
)
`
			tag, err := tx.Exec(ctx, sql, session.AccountId, applicationId, clientId)
			if err != nil {
				return err
			}

			deleted = tag.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !deleted {
			return echo.NewHTTPError(http.StatusNotFound, errors.New("the client does not exist"))
		}

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

func UpdateDevApplicationClientUserEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var applicationId, clientId, userId uuid.UUID
		if values, err := util.EchoAllParams(c, uuid.FromString, "app_id", "client_id", "user_id"); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		} else {
			applicationId, clientId, userId = values[0], values[1], values[2]
		}

		var update devAppClientUserUpdate
		if err := c.Bind(&update); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if !slices.Contains([]string{"APPROVED", "BLOCKED"}, update.ApprovalStatus) {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("invalid approval status"))
		}

		ctx := c.Request().Context()
		slog.InfoContext(
			ctx,
			"updating application client user",
			slog.String("application.id", applicationId.String()),
			slog.String("application.client.id", clientId.String()),
			slog.String("application.user.id", userId.String()),
			slog.String("application.client.user.status", update.ApprovalStatus),
		)

		var updated bool
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
UPDATE application_client_accounts
SET approval_status = $5, approval_request_message = $6
FROM (
	SELECT
		app_client_accounts.application_client_id,
		app_client_accounts.account_id
	FROM application_account_subs app_account_subs
	INNER JOIN application_client_accounts app_client_accounts
	USING (application_id, account_id)
	INNER JOIN applications app
	ON app_client_accounts.application_id = app.id
	WHERE app.account_id = $1
	AND app.id = $2
	AND app_client_accounts.application_client_id = $3
	AND app_account_subs.account_sub = $4
) AS app_client_account
WHERE application_client_accounts.application_client_id = app_client_account.application_client_id
AND application_client_accounts.account_id = app_client_account.account_id
`
			tag, err := tx.Exec(ctx, sql, session.AccountId, applicationId, clientId, userId, update.ApprovalStatus, update.ApprovalMessage)
			if err != nil {
				return err
			}

			updated = tag.RowsAffected() > 0
			return nil
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		if !updated {
			return echo.NewHTTPError(http.StatusNotFound, errors.New("no rows were updated"))
		}

		return c.JSON(http.StatusOK, update)
	})
}

func preprocessRedirectURIs(applicationId, clientId uuid.UUID, redirectURIs []string) []string {
	replacer := strings.NewReplacer("$application_id", applicationId.String(), "$client_id", clientId.String())

	r := make([]string, 0, len(redirectURIs))
	unq := make(util.Set[string])

	for _, redirectURI := range redirectURIs {
		redirectURI = replacer.Replace(redirectURI)
		if redirectURI != "" && unq.Add(redirectURI) {
			r = append(r, redirectURI)
		}
	}

	return r
}

func validateRedirectURIs(redirectURIs []string) error {
	var err error
	for _, redirectURI := range redirectURIs {
		originalRedirectURI := redirectURI
		if strings.ContainsRune(redirectURI, '*') {
			hashBytes := sha256.Sum256([]byte(redirectURI))
			hash := base64.RawURLEncoding.EncodeToString(hashBytes[:])
			redirectURI = strings.ReplaceAll(redirectURI, "*", "a"+hash+"z") // surround with "a" and "z" to ensure valid hostnames
		}

		u, parseErr := url.Parse(redirectURI)
		if parseErr != nil {
			err = errors.Join(err, parseErr)
			continue
		}

		if u.Scheme == "" {
			err = errors.Join(err, errors.New("invalid scheme"))
		} else if u.Hostname() == "localhost" {
			err = errors.Join(err, errors.New("localhost is not allowed"))
		} else if u.Hostname() == "" {
			err = errors.Join(err, errors.New("invalid hostname"))
		}

		if redirectURI != originalRedirectURI {
			if !util.URLMatch(originalRedirectURI, u) {
				err = errors.Join(err, errors.New("invalid pattern"))
			}
		}
	}

	return err
}

func generateClientSecret() (string, error) {
	const length = 64
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	r := make([]byte, length)
	b := make([]byte, 4)

	for i := range r {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}

		r[i] = chars[binary.BigEndian.Uint32(b)%uint32(len(chars))]
	}

	return string(r), nil
}

func encodeClientSecret(secret string) (string, error) {
	return service.EncodeArgon2id([]byte(secret))
}

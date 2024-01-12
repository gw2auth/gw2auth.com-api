package web

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/util"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
	"time"
)

type devApplicationCreate struct {
	DisplayName string `json:"displayName"`
}

type devApplicationForList struct {
	Id           uuid.UUID `json:"id"`
	CreationTime time.Time `json:"creationTime"`
	DisplayName  string    `json:"displayName"`
	ClientCount  uint64    `json:"clientCount"`
	UserCount    uint64    `json:"userCount"`
}

type devApplication struct {
	CreationTime time.Time                     `json:"creationTime"`
	DisplayName  string                        `json:"displayName"`
	Clients      []devApplicationClientForList `json:"clients"`
	ApiKeys      []devApplicationApiKeyForList `json:"apiKeys"`
}

type devApplicationClientForList struct {
	Id               uuid.UUID `json:"id"`
	CreationTime     time.Time `json:"creationTime"`
	DisplayName      string    `json:"displayName"`
	ApiVersion       uint32    `json:"apiVersion"`
	Type             string    `json:"type"`
	RequiresApproval bool      `json:"requiresApproval"`
}

type devApplicationApiKeyForList struct {
	Id          uuid.UUID         `json:"id"`
	Permissions []auth.Permission `json:"permissions"`
	NotBefore   time.Time         `json:"notBefore"`
	ExpiresAt   time.Time         `json:"expiresAt"`
}

type pagedResult[T any] struct {
	Items     []T    `json:"items"`
	NextToken string `json:"nextToken,omitempty"`
}

type cloudscapeQuery struct {
	Tokens    []cloudscapeQueryToken `json:"tokens"`
	Operation string                 `json:"operation"`
}

type cloudscapeQueryToken struct {
	PropertyKey string `json:"propertyKey"`
	Operator    string `json:"operator"`
	Value       string `json:"value"`
}

type devApplicationUser struct {
	UserId       uuid.UUID                 `json:"userId"`
	CreationTime time.Time                 `json:"creationTime"`
	Client       *devApplicationUserClient `json:"client,omitempty"`
}

type devApplicationUserClient struct {
	ClientId               uuid.UUID `json:"clientId"`
	ApprovalStatus         string    `json:"approvalStatus"`
	ApprovalRequestMessage string    `json:"approvalRequestMessage"`
	AuthorizedScopes       []string  `json:"authorizedScopes"`
}

type devApplicationApiKeyCreateRequest struct {
	Permissions []auth.Permission `json:"permissions"`
	ExpiresAt   string            `json:"expiresAt,omitempty"`
}

type devApplicationApiKeyCreateResponse struct {
	Id          uuid.UUID         `json:"id"`
	Key         string            `json:"key"`
	Permissions []auth.Permission `json:"permissions"`
	NotBefore   time.Time         `json:"notBefore"`
	ExpiresAt   time.Time         `json:"expiresAt"`
}

func CreateDevApplicationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var body devApplicationCreate
		if err := c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if body.DisplayName == "" || len(body.DisplayName) > 100 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("displayname must be between 1 and 100 characters"))
		}

		applicationId, err := uuid.NewV4()
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		ctx := c.Request().Context()
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
INSERT INTO applications
(id, account_id, creation_time, display_name)
VALUES
($1, $2, $3, $4)
`
			_, err := tx.Exec(ctx, sql, applicationId, session.AccountId, time.Now(), body.DisplayName)
			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, map[string]any{
			"id": applicationId,
		})
	})
}

func DeleteDevApplicationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		applicationId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()

		var deleted bool
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM applications
WHERE account_id = $1
AND id = $2
`
			tag, err := tx.Exec(ctx, sql, session.AccountId, applicationId)
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
			return echo.NewHTTPError(http.StatusNotFound, errors.New("the application does not exist"))
		}

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

func DevApplicationsEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		ctx := c.Request().Context()
		var results []devApplicationForList
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    apps.id,
    MAX(apps.creation_time),
    MAX(apps.display_name),
    COUNT(DISTINCT app_clients.id),
    COUNT(DISTINCT app_accs.account_id)
FROM applications apps
LEFT JOIN application_clients app_clients
ON apps.id = app_clients.application_id
LEFT JOIN application_accounts app_accs
ON apps.id = app_accs.application_id
WHERE apps.account_id = $1
GROUP BY apps.id
`
			rows, err := tx.Query(ctx, sql, session.AccountId)
			if err != nil {
				return err
			}

			results, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (devApplicationForList, error) {
				var app devApplicationForList
				return app, row.Scan(
					&app.Id,
					&app.CreationTime,
					&app.DisplayName,
					&app.ClientCount,
					&app.UserCount,
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

func DevApplicationEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var err error
		var applicationId uuid.UUID
		if applicationId, err = uuid.FromString(c.Param("id")); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		ctx := c.Request().Context()
		var result devApplication
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    MAX(app.creation_time),
    MAX(app.display_name),
    COALESCE(ARRAY_AGG(DISTINCT JSONB_BUILD_OBJECT(
		'id', app_clients.id,
		'creationTime', app_clients.creation_time,
        'displayName', app_clients.display_name,
        'apiVersion', app_clients.api_version,
        'type', app_clients.type,
        'requiresApproval', app_clients.requires_approval
	)) FILTER ( WHERE app_clients.id IS NOT NULL ), ARRAY[]::JSONB[]),
    COALESCE(ARRAY_AGG(DISTINCT JSONB_BUILD_OBJECT(
		'id', app_api_keys.id,
		'permissions', app_api_keys.permissions,
        'notBefore', app_api_keys.not_before,
        'expiresAt', app_api_keys.expires_at
	)) FILTER ( WHERE app_api_keys.id IS NOT NULL ), ARRAY[]::JSONB[])
FROM applications app
LEFT JOIN application_clients app_clients
ON app.id = app_clients.application_id
LEFT JOIN application_api_keys app_api_keys
ON app.id = app_api_keys.application_id
WHERE app.account_id = $1
AND app.id = $2
GROUP BY app.id
`
			return tx.QueryRow(ctx, sql, session.AccountId, applicationId).Scan(
				&result.CreationTime,
				&result.DisplayName,
				&result.Clients,
				&result.ApiKeys,
			)
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, result)
	})
}

func DevApplicationUsersEndpoint() echo.HandlerFunc {
	const defaultPageSize = 50

	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		applicationId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		additionalSQL := "TRUE"
		additionalParams := make([]any, 0)

		if qJson := c.QueryParam("query"); qJson != "" {
			var query cloudscapeQuery
			if err = json.Unmarshal([]byte(qJson), &query); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err)
			}

			// keep in sync with predefined query parameters
			const paramNum = 5
			if sql, params, err := translateQuery(paramNum, query); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err)
			} else {
				additionalSQL = sql
				additionalParams = params
			}
		}

		var t time.Time
		var pageSize uint32
		var offset uint32

		if nextToken := c.QueryParam("nextToken"); nextToken != "" {
			if err = parseNextToken(nextToken, &t, &pageSize, &offset); err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, err)
			}

			// in case anyone messes with the nextToken
			if pageSize < 1 || pageSize > 50 || t.Before(time.Now().Add(-time.Hour)) {
				return echo.NewHTTPError(http.StatusBadRequest, errors.New("pageSize or timestamp out of bounds"))
			}
		} else {
			t = time.Now().Add(-time.Second)
			pageSize = defaultPageSize
			offset = 0
		}

		ctx := c.Request().Context()
		var results []devApplicationUser
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, fmt.Sprintf("SET TRANSACTION AS OF SYSTEM TIME %d", t.UnixNano())); err != nil {
				return err
			}

			sql := `
SELECT
    app_account_subs.account_sub,
    app_accounts.creation_time,
    CASE WHEN app_client_accounts.account_id IS NOT NULL THEN
    JSONB_BUILD_OBJECT(
        'clientId', app_client_accounts.application_client_id,
        'approvalStatus', app_client_accounts.approval_status,
        'approvalRequestMessage', app_client_accounts.approval_request_message,
        'authorizedScopes', app_client_accounts.authorized_scopes
    )
	END 
FROM applications apps
INNER JOIN application_accounts app_accounts
ON apps.id = app_accounts.application_id
INNER JOIN application_account_subs app_account_subs
ON app_accounts.application_id = app_account_subs.application_id AND app_accounts.account_id = app_account_subs.account_id
LEFT JOIN application_client_accounts app_client_accounts
ON app_accounts.application_id = app_client_accounts.application_id AND app_accounts.account_id = app_client_accounts.account_id
WHERE apps.id = $1
AND apps.account_id = $2
`
			sql += fmt.Sprintf("AND ( %s ) OFFSET $3 LIMIT ($4 + 1)", additionalSQL)

			params := []any{applicationId, session.AccountId, offset, pageSize}
			params = append(params, additionalParams...)

			rows, err := tx.Query(ctx, sql, params...)
			if err != nil {
				return err
			}

			results, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (devApplicationUser, error) {
				var user devApplicationUser
				return user, row.Scan(
					&user.UserId,
					&user.CreationTime,
					&user.Client,
				)
			})

			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		nextToken := ""
		if len(results) > int(pageSize) {
			results = results[:pageSize]
			nextToken = buildNextToken(t, pageSize, offset+pageSize)
		}

		return c.JSON(http.StatusOK, pagedResult[devApplicationUser]{
			Items:     results,
			NextToken: nextToken,
		})
	})
}

func CreateDevApplicationAPIKeyEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		applicationId, err := uuid.FromString(c.Param("id"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		var body devApplicationApiKeyCreateRequest
		if err = c.Bind(&body); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		body.Permissions = auth.FilterPermissions(body.Permissions)
		if len(body.Permissions) < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("must have at least one permission"))
		}

		now := time.Now()
		var expiresAt time.Time
		if body.ExpiresAt == "" {
			expiresAt = now.Add(time.Hour * 24 * 365 * 100)
		} else if expiresAt, err = parseExpiresAt(body.ExpiresAt); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		}

		if now.Add(time.Hour).After(expiresAt) {
			return echo.NewHTTPError(http.StatusBadRequest, errors.New("must be valid for at least one hour"))
		}

		var apiKeyId uuid.UUID
		var apiKeyRaw string
		var apiKeyEncoded string

		if apiKeyId, err = uuid.NewV4(); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		if apiKeyRaw, err = generateClientSecret(); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		if apiKeyEncoded, err = service.EncodeArgon2id([]byte(apiKeyRaw)); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err)
		}

		ctx := c.Request().Context()
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
INSERT INTO application_api_keys
(id, application_id, key, permissions, not_before, expires_at)
VALUES
($1, $2, $3, $4, $5, $6)
`

			_, err := tx.Exec(ctx, sql, apiKeyId, applicationId, apiKeyEncoded, body.Permissions, now, expiresAt)
			return err
		})

		if err != nil {
			return util.NewEchoPgxHTTPError(err)
		}

		return c.JSON(http.StatusOK, devApplicationApiKeyCreateResponse{
			Id:          apiKeyId,
			Key:         apiKeyRaw,
			Permissions: body.Permissions,
			NotBefore:   now,
			ExpiresAt:   expiresAt,
		})
	})
}

func DeleteDevApplicationAPIKeyEndpoint() echo.HandlerFunc {
	return wrapAuthenticatedHandlerFunc(func(c echo.Context, rctx RequestContext, session auth.Session) error {
		var applicationId, keyId uuid.UUID
		if values, err := util.EchoAllParams(c, uuid.FromString, "app_id", "key_id"); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err)
		} else {
			applicationId, keyId = values[0], values[1]
		}

		ctx := c.Request().Context()
		var deleted bool
		err := rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			const sql = `
DELETE FROM application_api_keys
WHERE id = (
    SELECT app_api_keys.id
    FROM application_api_keys app_api_keys
    INNER JOIN applications apps
    ON app_api_keys.application_id = apps.id
    WHERE apps.account_id = $1
    AND apps.id = $2
    AND app_api_keys.id = $3
)
`
			tag, err := tx.Exec(ctx, sql, session.AccountId, applicationId, keyId)
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
			return echo.NewHTTPError(http.StatusNotFound, errors.New("the key does not exist"))
		}

		return c.JSON(http.StatusOK, map[string]string{})
	})
}

func translateQuery(paramNum int, query cloudscapeQuery) (string, []any, error) {
	propertyToSQL := map[string]string{
		"user_id":           "app_account_subs.account_sub",
		"creation_time":     "app_accounts.creation_time",
		"client_id":         "app_client_accounts.application_client_id",
		"approval_status":   "app_client_accounts.approval_status",
		"authorized_scopes": "app_client_accounts.authorized_scopes",
	}
	operationToSQL := map[string]string{
		"and": "AND",
		"or":  "OR",
	}
	operatorToSQL := map[string]util.SQLBinOp{
		"=":  util.SQLBinOpEQ,
		"!=": util.SQLBinOpNEQ,
		">":  util.SQLBinOpGT,
		">=": util.SQLBinOpGTE,
		"<":  util.SQLBinOpLT,
		"<=": util.SQLBinOpLTE,
		":":  util.SQLBinOpContains,
		"!:": util.SQLBinOpNotContains,
	}

	builder := util.NewSQLBuilder(paramNum)

	for _, tk := range query.Tokens {
		prop, ok := propertyToSQL[tk.PropertyKey]
		if !ok {
			return "", nil, errors.New("invalid property")
		}

		op, ok := operatorToSQL[tk.Operator]
		if !ok {
			return "", nil, errors.New("invalid operator")
		}

		value := any(tk.Value)
		if tk.PropertyKey == "authorized_scopes" {
			if tk.Value == "" {
				value = []string{}
			} else {
				value = strings.Split(tk.Value, ",")
			}
		}

		builder.Add(value, func(i int) string {
			return op(prop, util.SQLParam(i))
		})
	}

	operation, ok := operationToSQL[query.Operation]
	if !ok {
		return "", nil, errors.New("invalid operation")
	}

	expr, params := builder.Get()
	return strings.Join(expr, fmt.Sprintf(" %s ", operation)), params, nil
}

func parseNextToken(nextToken string, t *time.Time, pageSize *uint32, offset *uint32) error {
	b, err := base64.RawURLEncoding.DecodeString(nextToken)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(b)
	if buf.Len() < 9 { // 2 uint32 + at least 1 byte
		return errors.New("insufficient nextToken length")
	}

	if err = t.UnmarshalBinary(buf.Next(buf.Len() - 8)); err != nil {
		return err
	}

	*pageSize = binary.BigEndian.Uint32(buf.Next(4))
	*offset = binary.BigEndian.Uint32(buf.Next(4))

	return nil
}

func buildNextToken(t time.Time, pageSize uint32, offset uint32) string {
	b, _ := t.MarshalBinary()
	b = binary.BigEndian.AppendUint32(b, pageSize)
	b = binary.BigEndian.AppendUint32(b, offset)
	return base64.RawURLEncoding.EncodeToString(b)
}

func parseExpiresAt(s string) (time.Time, error) {
	layoutChain := []string{
		time.DateOnly,
		time.RFC3339,
	}

	var t time.Time
	var err error
	for _, layout := range layoutChain {
		var currErr error
		if t, currErr = time.Parse(layout, s); currErr == nil {
			return t, nil
		} else {
			err = errors.Join(err, currErr)
		}
	}

	return t, err
}

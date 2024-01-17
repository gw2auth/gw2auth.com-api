package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	crdbpgx "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgxv5"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const sessionCookieName = "BEARER"
const cfLatHeaderName = "Cloudfront-Viewer-Latitude"
const cfLngHeaderName = "Cloudfront-Viewer-Longitude"

type requestContextKey struct{}
type sessionContextKey struct{}
type applicationApiKeyContextKey struct{}

type SlogWithCtx struct {
	ctx context.Context
	log *slog.Logger
}

func (s SlogWithCtx) Debug(msg string, args ...any) {
	s.log.DebugContext(s.ctx, msg, args...)
}

func (s SlogWithCtx) Info(msg string, args ...any) {
	s.log.InfoContext(s.ctx, msg, args...)
}

func (s SlogWithCtx) Warn(msg string, args ...any) {
	s.log.WarnContext(s.ctx, msg, args...)
}

func (s SlogWithCtx) Error(msg string, args ...any) {
	s.log.ErrorContext(s.ctx, msg, args...)
}

type RequestContext interface {
	Log() SlogWithCtx
	ExecuteTx(ctx context.Context, txOptions pgx.TxOptions, fn func(pgx.Tx) error) error
}

type requestContext struct {
	log  SlogWithCtx
	pool *pgxpool.Pool
}

func (c *requestContext) Log() SlogWithCtx {
	return c.log
}

func (c *requestContext) ExecuteTx(ctx context.Context, txOptions pgx.TxOptions, fn func(pgx.Tx) error) error {
	return crdbpgx.ExecuteTx(ctx, c.pool, txOptions, fn)
}

func contextManipulatingMiddleware(fn func(c echo.Context) (context.Context, context.CancelFunc, error)) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			defer func() {
				c.SetRequest(req)
			}()

			ctx, cancel, err := fn(c)
			if err != nil {
				return err
			}

			if cancel != nil {
				defer cancel()
			}

			c.SetRequest(req.WithContext(ctx))

			return next(c)
		}
	}
}

func isSecure(c echo.Context) bool {
	if c.IsTLS() || c.Scheme() == "https" {
		return true
	}

	h := c.Request().Header
	if h.Get(echo.HeaderXForwardedProto) == "https" {
		return true
	} else if strings.Contains(h.Get("Forwarded"), "proto=https") {
		return true
	}

	return false
}

func deleteCookie(c echo.Context, cookie *http.Cookie) {
	cookie = &http.Cookie{
		Name:   cookie.Name,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
	c.SetCookie(cookie)
}

func Middleware(pool *pgxpool.Pool) echo.MiddlewareFunc {
	return contextManipulatingMiddleware(func(c echo.Context) (context.Context, context.CancelFunc, error) {
		ctx := c.Request().Context()
		return withRequestContext(ctx, pool), nil, nil
	})
}

func AuthenticatedMiddleware(conv *service.SessionJwtConverter) echo.MiddlewareFunc {
	tracer := otel.Tracer("github.com/gw2auth/gw2auth.com-api::AuthenticatedMiddleware", trace.WithInstrumentationVersion("v0.0.1"))

	updateCookie := func(c echo.Context, cookie *http.Cookie, newValue string, exp time.Time) {
		cookie = &http.Cookie{
			Name:     cookie.Name,
			Value:    newValue,
			Path:     "/",
			Expires:  exp,
			Secure:   cookie.Secure || isSecure(c),
			HttpOnly: true,
			SameSite: http.SameSiteStrictMode,
		}
		c.SetCookie(cookie)
	}

	onErr := func(c echo.Context, rctx RequestContext, cookie *http.Cookie, sessionId string, err error) error {
		rctx.Log().Info("authenticated handler failed to retrieve session", slog.String("err", err.Error()))

		if cookie != nil {
			deleteCookie(c, cookie)
		}

		if sessionId != "" {
			ctx := c.Request().Context()
			err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
				return auth.DeleteSession(ctx, tx, sessionId)
			})

			if err != nil {
				rctx.Log().Warn("failed to delete existing session", slog.String("err", err.Error()))
			}
		}

		return echo.NewHTTPError(http.StatusUnauthorized, map[string]string{})
	}

	return contextManipulatingMiddleware(func(c echo.Context) (context.Context, context.CancelFunc, error) {
		ctx := c.Request().Context()
		rctx, ok := ctx.Value(requestContextKey{}).(*requestContext)
		if !ok {
			return ctx, nil, echo.NewHTTPError(http.StatusInternalServerError, map[string]string{})
		}

		cookie, err := c.Cookie(sessionCookieName)
		if err != nil {
			return ctx, nil, onErr(c, rctx, cookie, "", err)
		}

		if err = cookie.Valid(); err != nil {
			return ctx, nil, onErr(c, rctx, cookie, "", err)
		}

		claims, iat, err := conv.ReadJWT(cookie.Value)
		if err != nil {
			return ctx, nil, onErr(c, rctx, cookie, "", err)
		}

		var sessionMetadata auth.SessionMetadata
		sessionMetadata.Lat, err = strconv.ParseFloat(c.Request().Header.Get(cfLatHeaderName), 64)
		if err != nil {
			return ctx, nil, onErr(c, rctx, cookie, claims.SessionId, err)
		}

		sessionMetadata.Lng, err = strconv.ParseFloat(c.Request().Header.Get(cfLngHeaderName), 64)
		if err != nil {
			return ctx, nil, onErr(c, rctx, cookie, claims.SessionId, err)
		}

		var session auth.Session
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
			return auth.LoadAndUpdateSession(ctx, tx, claims.SessionId, claims.EncryptionKey, iat, sessionMetadata, &session)
		})

		if err != nil {
			return ctx, nil, onErr(c, rctx, cookie, claims.SessionId, err)
		}

		jwtStr, err := conv.WriteJWT(claims, session.ExpirationTime)
		if err != nil {
			return ctx, nil, onErr(c, rctx, cookie, claims.SessionId, err)
		}

		updateCookie(c, cookie, jwtStr, session.ExpirationTime)

		// this cookie should no longer be persisted if the request is already authenticated
		if cookie, err := c.Cookie("REDIRECT_URI"); err == nil {
			deleteCookie(c, cookie)
		}

		ctx, span := tracer.Start(
			ctx,
			"Authenticated",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("session.id", session.Id),
				attribute.String("session.account.id", session.AccountId.String()),
			),
		)
		return withSession(ctx, session), func() { span.End() }, nil
	})
}

func ApplicationAPIKeyAuthenticatedMiddleware() echo.MiddlewareFunc {
	tracer := otel.Tracer("github.com/gw2auth/gw2auth.com-api::APIKeyAuthenticatedMiddleware", trace.WithInstrumentationVersion("v0.0.1"))

	return contextManipulatingMiddleware(func(c echo.Context) (context.Context, context.CancelFunc, error) {
		ctx := c.Request().Context()
		rctx, ok := ctx.Value(requestContextKey{}).(*requestContext)
		if !ok {
			return ctx, nil, echo.NewHTTPError(http.StatusInternalServerError, map[string]string{})
		}

		var keyIdRaw, keyRaw string
		var err error

		if keyIdRaw, keyRaw, ok = c.Request().BasicAuth(); !ok {
			return ctx, nil, echo.NewHTTPError(http.StatusUnauthorized)
		}

		var keyId uuid.UUID
		if keyId, err = uuid.FromString(keyIdRaw); err != nil {
			return ctx, nil, echo.NewHTTPError(http.StatusUnauthorized)
		}

		var apiKey auth.ApiKey
		var apiKeyEncoded string
		err = rctx.ExecuteTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			const sql = `
SELECT
    k.key,
    k.id,
    k.application_id,
    k.permissions,
    k.not_before,
    k.expires_at,
    app.account_id
FROM application_api_keys k
INNER JOIN applications app
ON k.application_id = app.id
WHERE k.id = $1
`

			return tx.QueryRow(ctx, sql, keyId).Scan(
				&apiKeyEncoded,
				&apiKey.Id,
				&apiKey.ApplicationId,
				&apiKey.Permissions,
				&apiKey.NotBefore,
				&apiKey.ExpiresAt,
				&apiKey.AccountId,
			)
		})

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ctx, nil, echo.NewHTTPError(http.StatusUnauthorized)
			}

			return ctx, nil, echo.NewHTTPError(http.StatusInternalServerError)
		}

		now := time.Now()
		if now.Before(apiKey.NotBefore) || now.After(apiKey.ExpiresAt) || len(apiKey.Permissions) < 1 || !service.VerifyArgon2id(apiKeyEncoded, []byte(keyRaw)) {
			return ctx, nil, echo.NewHTTPError(http.StatusUnauthorized)
		}

		ctx, span := tracer.Start(
			ctx,
			"APIKeyAuthenticated",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("api_key.id", apiKey.Id.String()),
				attribute.String("api_key.application.id", apiKey.ApplicationId.String()),
				attribute.String("api_key.account.id", apiKey.AccountId.String()),
			),
		)

		return withApiKey(ctx, apiKey), func() { span.End() }, nil
	})
}

func ApplicationAPIKeyPermissionMiddleware(requiredPermissions ...auth.Permission) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			apiKey, ok := c.Request().Context().Value(applicationApiKeyContextKey{}).(auth.ApiKey)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized)
			}

			for _, perm := range requiredPermissions {
				if !slices.Contains(apiKey.Permissions, perm) {
					return echo.NewHTTPError(http.StatusUnauthorized)
				}
			}

			return next(c)
		}
	}
}

func CSRFMiddleware() echo.MiddlewareFunc {
	ignoreMethods := map[string]bool{
		http.MethodGet:     true,
		http.MethodHead:    true,
		http.MethodOptions: true,
		http.MethodTrace:   true,
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			token := ""

			if cookie, err := c.Cookie("XSRF-TOKEN"); err == nil {
				token = cookie.Value
			} else {
				b := make([]byte, 32)
				_, err = rand.Read(b)
				if err != nil {
					return echo.NewHTTPError(http.StatusInternalServerError, "could not generate csrf token")
				}

				token = base64.RawURLEncoding.EncodeToString(b)
				c.SetCookie(&http.Cookie{
					Name:     "XSRF-TOKEN",
					Value:    token,
					Path:     "/",
					MaxAge:   0,
					Secure:   isSecure(c),
					HttpOnly: false,
					SameSite: http.SameSiteStrictMode,
				})
			}

			if v, _ := ignoreMethods[req.Method]; v {
				return next(c)
			}

			clientToken := req.Header.Get("X-Xsrf-Token")
			if clientToken == "" {
				return echo.NewHTTPError(http.StatusBadRequest, "missing csrf token in request header")
			} else if clientToken != token {
				return echo.NewHTTPError(http.StatusForbidden, "invalid csrf token")
			}

			return next(c)
		}
	}
}

func DeleteHistoricalCookiesMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			for _, name := range []string{"cookieconsent_status", "JSESSIONID"} {
				if cookie, err := c.Cookie(name); err == nil {
					deleteCookie(c, cookie)
				}
			}

			return next(c)
		}
	}
}

type HandlerFunc func(c echo.Context, rctx RequestContext) error
type AuthenticatedHandlerFunc func(c echo.Context, rctx RequestContext, session auth.Session) error
type ApiKeyAuthenticatedHandlerFunc func(c echo.Context, rctx RequestContext, apiKey auth.ApiKey) error

func wrapHandlerFunc(fn HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := c.Request().Context()
		rctx, ok := ctx.Value(requestContextKey{}).(*requestContext)
		if !ok {
			return echo.NewHTTPError(http.StatusInternalServerError, map[string]string{})
		}

		return fn(c, rctx)
	}
}

func wrapAuthenticatedHandlerFunc(fn AuthenticatedHandlerFunc) echo.HandlerFunc {
	return wrapHandlerFunc(func(c echo.Context, rctx RequestContext) error {
		ctx := c.Request().Context()
		session, ok := ctx.Value(sessionContextKey{}).(auth.Session)
		if !ok {
			rctx.Log().Warn("session not found in context on authenticated handler")
			return echo.NewHTTPError(http.StatusInternalServerError, map[string]string{})
		}

		return fn(c, rctx, session)
	})
}

func wrapApiKeyAuthenticatedHandlerFunc(fn ApiKeyAuthenticatedHandlerFunc) echo.HandlerFunc {
	return wrapHandlerFunc(func(c echo.Context, rctx RequestContext) error {
		ctx := c.Request().Context()
		apiKey, ok := ctx.Value(applicationApiKeyContextKey{}).(auth.ApiKey)
		if !ok {
			rctx.Log().Warn("api key not found in context on authenticated handler")
			return echo.NewHTTPError(http.StatusInternalServerError, map[string]string{})
		}

		return fn(c, rctx, apiKey)
	})
}

func withRequestContext(ctx context.Context, pool *pgxpool.Pool) context.Context {
	return context.WithValue(ctx, requestContextKey{}, &requestContext{
		log: SlogWithCtx{
			ctx: ctx,
			log: slog.Default(),
		},
		pool: pool,
	})
}

func withSession(ctx context.Context, session auth.Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, session)
}

func withApiKey(ctx context.Context, apiKey auth.ApiKey) context.Context {
	return context.WithValue(ctx, applicationApiKeyContextKey{}, apiKey)
}

package main

import (
	"context"
	"crypto/rsa"
	"github.com/exaring/otelpgx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/gw2auth/gw2auth.com-api/service/auth"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/gw2auth/gw2auth.com-api/web"
	pgxuuid "github.com/jackc/pgx-gofrs-uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-lambda-go/otellambda"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"net/http"
)

type Secrets struct {
	DatabaseURL           string `json:"databaseURL"`
	SessionRSAPublicKid1  string `json:"sessionRSAPublicKid1"`
	SessionRSAPublicKid2  string `json:"sessionRSAPublicKid2"`
	SessionRSAPrivateKid2 string `json:"sessionRSAPrivateKid2"`
	SessionRSAPublicPEM1  string `json:"sessionRSAPublicPEM1"`
	SessionRSAPublicPEM2  string `json:"sessionRSAPublicPEM2"`
	SessionRSAPrivatePEM2 string `json:"sessionRSAPrivatePEM2"`
}

type Option func(app *echo.Echo)

func newPgx(secrets Secrets) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(secrets.DatabaseURL)
	if err != nil {
		return nil, err
	}

	config.ConnConfig.Tracer = otelpgx.NewTracer()
	config.ConnConfig.RuntimeParams["application_name"] = "api.gw2auth.com"
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxuuid.Register(conn.TypeMap())
		return nil
	}

	return pgxpool.NewWithConfig(context.Background(), config)
}

func newConv(secrets Secrets) (*service.SessionJwtConverter, error) {
	pub1, err := jwt.ParseRSAPublicKeyFromPEM([]byte(secrets.SessionRSAPublicPEM1))
	if err != nil {
		return nil, err
	}

	pub2, err := jwt.ParseRSAPublicKeyFromPEM([]byte(secrets.SessionRSAPublicPEM2))
	if err != nil {
		return nil, err
	}

	priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(secrets.SessionRSAPrivatePEM2))
	if err != nil {
		return nil, err
	}

	pub := map[string]*rsa.PublicKey{
		secrets.SessionRSAPublicKid1: pub1,
		secrets.SessionRSAPublicKid2: pub2,
	}

	return service.NewSessionJwtConverter(secrets.SessionRSAPrivateKid2, priv, pub), nil
}

func newHttpClient() *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}
}

func newGw2ApiClient(httpClient *http.Client) *gw2.ApiClient {
	return gw2.NewApiClient(httpClient, "https://api.guildwars2.com")
}

func newEchoServer(pool *pgxpool.Pool, httpClient *http.Client, gw2ApiClient *gw2.ApiClient, conv *service.SessionJwtConverter, options ...Option) *echo.Echo {
	app := echo.New()

	for _, opt := range options {
		opt(app)
	}

	app.Use(
		otelecho.Middleware("api.gw2auth.com"),
		web.Middleware(pool),
	)

	// region UI
	uiGroup := app.Group("/api-v2", web.DeleteHistoricalCookiesMiddleware(), web.CSRFMiddleware())
	authMw := web.AuthenticatedMiddleware(conv)

	uiGroup.GET("/account", web.AccountEndpoint(), authMw)
	uiGroup.DELETE("/account", web.DeleteAccountEndpoint(), authMw)
	uiGroup.DELETE("/account/federation", web.DeleteAccountFederationEndpoint(), authMw)
	uiGroup.DELETE("/account/session", web.DeleteAccountFederationSessionEndpoint(), authMw)

	uiGroup.GET("/application/summary", web.AppSummaryEndpoint())
	uiGroup.GET("/authinfo", web.AuthInfoEndpoint(), authMw)
	uiGroup.GET("/gw2account", web.Gw2AccountsEndpoint(), authMw)
	uiGroup.GET("/gw2account/:id", web.Gw2AccountEndpoint(), authMw)
	uiGroup.PATCH("/gw2account/:id", web.UpdateGw2AccountEndpoint(), authMw)

	addOrUpdateTokenEndpoint := web.AddOrUpdateApiTokenEndpoint(gw2ApiClient)
	uiGroup.PUT("/gw2apitoken", addOrUpdateTokenEndpoint, authMw)
	uiGroup.PUT("/gw2apitoken/:id", addOrUpdateTokenEndpoint, authMw)
	uiGroup.PATCH("/gw2apitoken", addOrUpdateTokenEndpoint, authMw)
	uiGroup.PATCH("/gw2apitoken/:id", addOrUpdateTokenEndpoint, authMw)
	uiGroup.DELETE("/gw2apitoken/:id", web.DeleteApiTokenEndpoint(), authMw)
	uiGroup.GET("/gw2apitoken/verification", web.ApiTokenVerificationEndpoint(), authMw)

	uiGroup.GET("/application", web.UserApplicationsEndpoint(), authMw)
	uiGroup.GET("/application/:id", web.UserApplicationEndpoint(), authMw)
	uiGroup.DELETE("/application/:id", web.DeleteUserApplicationEndpoint(), authMw)

	uiGroup.GET("/verification/active", web.VerificationActiveEndpoint(), authMw)
	uiGroup.GET("/verification/pending", web.VerificationPendingEndpoint(), authMw)

	uiGroup.PUT("/dev/application", web.CreateDevApplicationEndpoint(), authMw)
	uiGroup.GET("/dev/application", web.DevApplicationsEndpoint(), authMw)
	uiGroup.GET("/dev/application/:id", web.DevApplicationEndpoint(), authMw)
	uiGroup.DELETE("/dev/application/:id", web.DeleteDevApplicationEndpoint(), authMw)
	uiGroup.GET("/dev/application/:id/user", web.DevApplicationUsersEndpoint(), authMw)
	uiGroup.PUT("/dev/application/:id/client", web.CreateDevApplicationClientEndpoint(), authMw)
	uiGroup.GET("/dev/application/:app_id/client/:client_id", web.DevApplicationClientEndpoint(), authMw)
	uiGroup.DELETE("/dev/application/:app_id/client/:client_id", web.DeleteDevApplicationClientEndpoint(), authMw)
	uiGroup.POST("/dev/application/:app_id/client/:client_id/secret", web.RegenerateDevApplicationClientSecretEndpoint(), authMw)
	uiGroup.PUT("/dev/application/:app_id/client/:client_id/redirecturi", web.UpdateDevApplicationClientRedirectURIsEndpoint(), authMw)
	uiGroup.PATCH("/dev/application/:app_id/client/:client_id/user/:user_id", web.UpdateDevApplicationClientUserEndpoint(), authMw)
	uiGroup.PUT("/dev/application/:id/apikey", web.CreateDevApplicationAPIKeyEndpoint(), authMw)
	uiGroup.DELETE("/dev/application/:app_id/apikey/:key_id", web.DeleteDevApplicationAPIKeyEndpoint(), authMw)

	uiGroup.GET("/notifications", web.NotificationsEndpoint(httpClient))
	// endregion

	// region application api
	applicationAPIGroup := app.Group("/api-app", web.ApplicationAPIKeyAuthenticatedMiddleware())
	applicationAPIGroup.PATCH("/application/client/:client_id/redirecturi", web.ModifyDevApplicationClientRedirectURIsEndpoint(), web.ApplicationAPIKeyPermissionMiddleware(auth.PermissionClientModify))
	// endregion

	return app
}

func WithEchoServer(ctx context.Context, fn func(ctx context.Context, app *echo.Echo) error, options ...Option) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	secrets, err := loadSecrets(ctx)
	if err != nil {
		return err
	}

	return withPgx(secrets, func(pool *pgxpool.Pool) error {
		return withConv(secrets, func(conv *service.SessionJwtConverter) error {
			httpClient := newHttpClient()
			return fn(ctx, newEchoServer(pool, httpClient, newGw2ApiClient(httpClient), conv, options...))
		})
	})
}

func WithFlusher(flusher otellambda.Flusher) Option {
	return func(app *echo.Echo) {
		app.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				defer flusher.ForceFlush(c.Request().Context())
				return next(c)
			}
		})
	}
}

func withPgx(secrets Secrets, fn func(pool *pgxpool.Pool) error) error {
	pool, err := newPgx(secrets)
	if err != nil {
		return err
	}

	defer pool.Close()

	return fn(pool)
}

func withConv(secrets Secrets, fn func(conv *service.SessionJwtConverter) error) error {
	conv, err := newConv(secrets)
	if err != nil {
		return err
	}

	return fn(conv)
}

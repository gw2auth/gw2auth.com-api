package web

import (
	"fmt"
	"github.com/gw2auth/gw2auth.com-api/internal/test"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAccountAll(t *testing.T) {
	test.RunAll(t, dbScope, map[string]map[string]test.Fn{
		"AccountEndpoint": {
			"unauthorized": testAccountEndpointUnauthorized,
			"simple":       testAccountEndpointSimple,
		},
		"DeleteAccountEndpoint": {
			"unauthorized": testDeleteAccountEndpointUnauthorized,
			"simple":       testDeleteAccountEndpointSimple,
		},
		"DeleteAccountFederationEndpoint": {
			"unauthorized":       testDeleteAccountFederationEndpointUnauthorized,
			"current federation": testDeleteAccountFederationEndpointCurrentFederation,
		},
		"DeleteAccountFederationSessionEndpoint": {
			"unauthorized":    testDeleteAccountFederationSessionEndpointUnauthorized,
			"current session": testDeleteAccountFederationSessionEndpointCurrentSession,
		},
	})
}

func testAccountEndpointUnauthorized(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	e := newEchoWithMiddleware(pool, conv)
	e.GET("/", AccountEndpoint())
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodGet, "/", nil))
}

func testAccountEndpointSimple(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	test.CreateAccountAndFederation(t, pool, test.NewUUID(t), "google", "GoogleA", time.Now())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	_, sessionId, creationTime, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleB")

	e := newEchoWithMiddleware(pool, conv)
	e.GET("/", AccountEndpoint())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(
		t,
		fmt.Sprintf(`{
	"federations": [
		{"issuer": "google", "idAtIssuer": "GoogleB"}
	],
	"sessions":[
		{"issuer": "google", "idAtIssuer": "GoogleB", "id": "%s", "creationTime": "%s"}
	]
}`, sessionId, creationTime.UTC().Truncate(time.Microsecond).Format(time.RFC3339Nano)),
		rec.Body.String(),
	)
}

func testDeleteAccountEndpointUnauthorized(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	e := newEchoWithMiddleware(pool, conv)
	e.DELETE("/", DeleteAccountEndpoint())
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodDelete, "/", nil))
}

func testDeleteAccountEndpointSimple(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	otherAccountId := test.NewUUID(t)
	test.CreateAccountAndFederation(t, pool, otherAccountId, "google", "GoogleA", time.Now())

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	accountId, _, _, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleB")

	e := newEchoWithMiddleware(pool, conv)
	e.DELETE("/", DeleteAccountEndpoint())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.Bytes())

	test.MustNotExist(
		t,
		pool,
		"SELECT TRUE FROM accounts WHERE id = $1",
		accountId,
	)

	test.MustExist(
		t,
		pool,
		"SELECT TRUE FROM accounts WHERE id = $1",
		otherAccountId,
	)
}

func testDeleteAccountFederationEndpointUnauthorized(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	e := newEchoWithMiddleware(pool, conv)
	e.DELETE("/", DeleteAccountFederationEndpoint())
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodDelete, "/", nil))
}

func testDeleteAccountFederationEndpointCurrentFederation(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	test.CreateAccountAndFederation(t, pool, test.NewUUID(t), "google", "GoogleA", time.Now())

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	test.AddQuery(req, "issuer", "google", "idAtIssuer", "GoogleB")
	_, _, _, _ = test.Authenticated(t, req, pool, conv, "google", "GoogleB")

	e := newEchoWithMiddleware(pool, conv)
	e.DELETE("/", DeleteAccountFederationEndpoint())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	test.MustExist(
		t,
		pool,
		"SELECT TRUE FROM account_federations WHERE issuer = $1 AND id_at_issuer = $2",
		"google",
		"GoogleB",
	)

	test.MustExist(
		t,
		pool,
		"SELECT TRUE FROM account_federations WHERE issuer = $1 AND id_at_issuer = $2",
		"google",
		"GoogleA",
	)
}

func testDeleteAccountFederationSessionEndpointUnauthorized(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	e := newEchoWithMiddleware(pool, conv)
	e.DELETE("/", DeleteAccountFederationSessionEndpoint())
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodDelete, "/", nil))
}

func testDeleteAccountFederationSessionEndpointCurrentSession(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	test.CreateAccountAndFederation(t, pool, test.NewUUID(t), "google", "GoogleA", time.Now())

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	_, sessionId, _, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleB")
	test.AddQuery(req, "id", sessionId)

	e := newEchoWithMiddleware(pool, conv)
	e.DELETE("/", DeleteAccountFederationSessionEndpoint())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	test.MustExist(
		t,
		pool,
		"SELECT TRUE FROM account_federation_sessions WHERE id = $1",
		sessionId,
	)
}

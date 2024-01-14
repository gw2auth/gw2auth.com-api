package web

import (
	"encoding/json"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/internal/test"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGw2ApiTokenAll(t *testing.T) {
	test.RunAll(t, dbScope, map[string]map[string]test.Fn{
		"AddOrUpdateApiTokenEndpoint": {
			"unauthorized":                       testAddOrUpdateApiTokenEndpointUnauthorized,
			"add invalid token":                  testAddOrUpdateApiTokenEndpointAddInvalidApiToken,
			"add happycase":                      testAddOrUpdateApiTokenEndpointAddHappycase,
			"add with verification":              testAddOrUpdateApiTokenEndpointAddWithVerification,
			"update happycase":                   testAddOrUpdateApiTokenEndpointUpdateHappycase,
			"verified for other gw2auth account": testAddOrUpdateApiTokenEndpointVerifiedForOtherGw2AuthAccount,
		},
	})
}

func testAddOrUpdateApiTokenEndpointUnauthorized(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	e := newEchoWithMiddleware(pool, conv)
	e.PUT("/", AddOrUpdateApiTokenEndpoint(nil))
	e.PUT("/:id", AddOrUpdateApiTokenEndpoint(nil))
	e.PATCH("/", AddOrUpdateApiTokenEndpoint(nil))
	e.PATCH("/:id", AddOrUpdateApiTokenEndpoint(nil))

	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodPut, "/", nil))
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodPut, "/93df54c6-78f7-e111-809d-78e7d1936ef0", nil))
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodPatch, "/", nil))
	test.MustUnauthorized(t, e, httptest.NewRequest(http.MethodPatch, "/93df54c6-78f7-e111-809d-78e7d1936ef0", nil))
}

func testAddOrUpdateApiTokenEndpointAddInvalidApiToken(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	gw2AccountId := uuid.FromStringOrNil("93df54c6-78f7-e111-809d-78e7d1936ef0")

	mux := http.NewServeMux()
	prepareMuxForAccountRequest(mux, "testApiToken", gw2AccountId, "Felix.9127")
	prepareMuxForTokenInfoRequest(mux, "testApiToken", "TokenName", gw2.PermissionAccount)

	assert.NoError(t, test.WithGw2ApiClient(mux, func(gw2ApiClient *gw2.ApiClient) error {
		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"apiToken": "testApiToken2"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		_, _, _, _ = test.Authenticated(t, req, pool, conv, "google", "GoogleA")

		e := newEchoWithMiddleware(pool, conv)
		e.PUT("/", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.JSONEq(t, `{"message": "invalid api token\nstatus=401 response=[]"}`, rec.Body.String())

		return nil
	}))
}

func testAddOrUpdateApiTokenEndpointAddHappycase(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	gw2AccountId := uuid.FromStringOrNil("93df54c6-78f7-e111-809d-78e7d1936ef0")

	mux := http.NewServeMux()
	prepareMuxForAccountRequest(mux, "testApiToken", gw2AccountId, "Felix.9127")
	prepareMuxForTokenInfoRequest(mux, "testApiToken", "TokenName", gw2.PermissionAccount)

	assert.NoError(t, test.WithGw2ApiClient(mux, func(gw2ApiClient *gw2.ApiClient) error {
		e := newEchoWithMiddleware(pool, conv)
		e.PUT("/", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
		e.PUT("/:id", AddOrUpdateApiTokenEndpoint(gw2ApiClient))

		testFn := func(t *testing.T, path string) {
			req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"apiToken": "testApiToken"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			accountId, _, _, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleA")

			start := time.Now()
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			end := time.Now()

			assert.Equal(t, http.StatusOK, rec.Code)

			var body apiTokenAddOrUpdateResponse
			if assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body)) {
				assert.True(t, body.CreationTime.Equal(start) || body.CreationTime.After(start))
				assert.True(t, body.CreationTime.Equal(end) || body.CreationTime.Before(end))
				assert.Equal(
					t,
					apiTokenAddOrUpdateResponse{
						Value:        "testApiToken",
						CreationTime: body.CreationTime,
						Permissions:  []gw2.Permission{gw2.PermissionAccount},
						Verified:     false,
					},
					body,
				)

				test.MustExist(
					t,
					pool,
					`
SELECT TRUE
FROM gw2_accounts gw2_acc
INNER JOIN gw2_account_api_tokens tk
USING (account_id, gw2_account_id)
WHERE gw2_acc.account_id = $1
AND gw2_acc.gw2_account_id = $2
`,
					accountId,
					gw2AccountId,
				)
			}
		}

		for _, path := range []string{"/", "/" + url.PathEscape(gw2AccountId.String())} {
			t.Run(path, func(t *testing.T) {
				t.Cleanup(func() {
					if !assert.NoError(t, truncateTablesFn()) {
						t.FailNow()
					}
				})

				testFn(t, path)
			})
		}

		return nil
	}))
}

func testAddOrUpdateApiTokenEndpointUpdateHappycase(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	gw2AccountId := uuid.FromStringOrNil("93df54c6-78f7-e111-809d-78e7d1936ef0")

	mux := http.NewServeMux()
	prepareMuxForAccountRequest(mux, "testApiToken", gw2AccountId, "Felix.9127")
	prepareMuxForTokenInfoRequest(mux, "testApiToken", "TokenName", gw2.PermissionAccount)

	assert.NoError(t, test.WithGw2ApiClient(mux, func(gw2ApiClient *gw2.ApiClient) error {
		e := newEchoWithMiddleware(pool, conv)
		e.PATCH("/", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
		e.PATCH("/:id", AddOrUpdateApiTokenEndpoint(gw2ApiClient))

		testFn := func(t *testing.T, path string) {
			req := httptest.NewRequest(http.MethodPatch, path, strings.NewReader(`{"apiToken": "testApiToken"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			accountId, _, _, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleA")

			test.CreateGw2Account(t, pool, accountId, gw2AccountId, "Felix.9127", "Felix.9127")
			test.CreateGw2ApiToken(t, pool, accountId, gw2AccountId, "oldApiToken", []gw2.Permission{})

			start := time.Now()
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			end := time.Now()

			assert.Equal(t, http.StatusOK, rec.Code)

			var body apiTokenAddOrUpdateResponse
			if assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body)) {
				assert.True(t, body.CreationTime.Equal(start) || body.CreationTime.After(start))
				assert.True(t, body.CreationTime.Equal(end) || body.CreationTime.Before(end))
				assert.Equal(
					t,
					apiTokenAddOrUpdateResponse{
						Value:        "testApiToken",
						CreationTime: body.CreationTime,
						Permissions:  []gw2.Permission{gw2.PermissionAccount},
						Verified:     false,
					},
					body,
				)

				test.MustExist(
					t,
					pool,
					`
SELECT TRUE
FROM gw2_accounts gw2_acc
INNER JOIN gw2_account_api_tokens tk
USING (account_id, gw2_account_id)
WHERE gw2_acc.account_id = $1
AND gw2_acc.gw2_account_id = $2
AND tk.gw2_api_token = $3
AND tk.gw2_api_permissions_bit_set = $4
`,
					accountId,
					gw2AccountId,
					"testApiToken",
					gw2.PermissionsToBitSet([]gw2.Permission{gw2.PermissionAccount}),
				)
			}
		}

		for _, path := range []string{"/", "/" + url.PathEscape(gw2AccountId.String())} {
			t.Run(path, func(t *testing.T) {
				t.Cleanup(func() {
					if !assert.NoError(t, truncateTablesFn()) {
						t.FailNow()
					}
				})

				testFn(t, path)
			})
		}

		return nil
	}))
}

func testAddOrUpdateApiTokenEndpointAddWithVerification(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	otherAccountId := test.NewUUID(t)
	gw2AccountId := uuid.FromStringOrNil("93df54c6-78f7-e111-809d-78e7d1936ef0")

	testFn := func(t *testing.T, path string) {
		// create verification and token for a different account
		test.CreateAccount(t, pool, otherAccountId, time.Now())
		test.CreateGw2Account(t, pool, otherAccountId, gw2AccountId, "Felix.9127", "Felix.9127")
		test.CreateGw2ApiToken(t, pool, otherAccountId, gw2AccountId, "otherTestApiToken", []gw2.Permission{gw2.PermissionAccount})
		test.CreateGw2AccountVerification(t, pool, otherAccountId, gw2AccountId)

		// prepare request
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"apiToken": "testApiToken"}`))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		accountId, sessionId, _, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleA")

		// prepare gw2 api to return the expected token name for the test token
		mux := http.NewServeMux()
		prepareMuxForAccountRequest(mux, "testApiToken", gw2AccountId, "Felix.9127")
		prepareMuxForTokenInfoRequest(mux, "testApiToken", verificationTokenNameForSession(sessionId), gw2.PermissionAccount)

		assert.NoError(t, test.WithGw2ApiClient(mux, func(gw2ApiClient *gw2.ApiClient) error {
			e := newEchoWithMiddleware(pool, conv)
			e.PUT("/", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
			e.PUT("/:id", AddOrUpdateApiTokenEndpoint(gw2ApiClient))

			start := time.Now()
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			end := time.Now()

			assert.Equal(t, http.StatusOK, rec.Code)

			var body apiTokenAddOrUpdateResponse
			if assert.NoError(t, json.NewDecoder(rec.Body).Decode(&body)) {
				assert.True(t, body.CreationTime.Equal(start) || body.CreationTime.After(start))
				assert.True(t, body.CreationTime.Equal(end) || body.CreationTime.Before(end))
				assert.Equal(
					t,
					apiTokenAddOrUpdateResponse{
						Value:        "testApiToken",
						CreationTime: body.CreationTime,
						Permissions:  []gw2.Permission{gw2.PermissionAccount},
						Verified:     true,
					},
					body,
				)

				test.MustExist(
					t,
					pool,
					`
SELECT TRUE
FROM gw2_accounts gw2_acc
INNER JOIN gw2_account_api_tokens tk
USING (account_id, gw2_account_id)
INNER JOIN gw2_account_verifications gw2_acc_ver
USING (account_id, gw2_account_id)
WHERE gw2_acc.account_id = $1
AND gw2_acc.gw2_account_id = $2
AND tk.gw2_api_token = $3
AND tk.gw2_api_permissions_bit_set = $4
`,
					accountId,
					gw2AccountId,
					"testApiToken",
					gw2.PermissionsToBitSet([]gw2.Permission{gw2.PermissionAccount}),
				)

				test.MustNotExist(
					t,
					pool,
					`
SELECT TRUE
FROM gw2_account_api_tokens
WHERE account_id = $1
AND gw2_account_id = $2
`,
					otherAccountId,
					gw2AccountId,
				)
			}

			return nil
		}))
	}

	for _, path := range []string{"/", "/" + url.PathEscape(gw2AccountId.String())} {
		t.Run(path, func(t *testing.T) {
			t.Cleanup(func() {
				if !assert.NoError(t, truncateTablesFn()) {
					t.FailNow()
				}
			})

			testFn(t, path)
		})
	}
}

func testAddOrUpdateApiTokenEndpointVerifiedForOtherGw2AuthAccount(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error) {
	otherAccountId := test.NewUUID(t)
	gw2AccountId := uuid.FromStringOrNil("93df54c6-78f7-e111-809d-78e7d1936ef0")

	mux := http.NewServeMux()
	prepareMuxForAccountRequest(mux, "testApiToken", gw2AccountId, "Felix.9127")
	prepareMuxForTokenInfoRequest(mux, "testApiToken", "TokenName", gw2.PermissionAccount)

	assert.NoError(t, test.WithGw2ApiClient(mux, func(gw2ApiClient *gw2.ApiClient) error {
		e := newEchoWithMiddleware(pool, conv)
		e.PUT("/", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
		e.PUT("/:id", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
		e.PATCH("/", AddOrUpdateApiTokenEndpoint(gw2ApiClient))
		e.PATCH("/:id", AddOrUpdateApiTokenEndpoint(gw2ApiClient))

		testFn := func(t *testing.T, method, path string) {
			test.CreateAccount(t, pool, otherAccountId, time.Now())
			test.CreateGw2Account(t, pool, otherAccountId, gw2AccountId, "Felix.9127", "Felix.9127")
			test.CreateGw2AccountVerification(t, pool, otherAccountId, gw2AccountId)

			req := httptest.NewRequest(method, path, strings.NewReader(`{"apiToken": "testApiToken"}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			accountId, _, _, _ := test.Authenticated(t, req, pool, conv, "google", "GoogleA")

			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotAcceptable, rec.Code)
			assert.JSONEq(t, `{"message": "the gw2account is already verified for another gw2auth account"}`, rec.Body.String())

			test.MustNotExist(
				t,
				pool,
				`
SELECT TRUE
FROM gw2_accounts
WHERE account_id = $1
AND gw2_account_id = $2
`,
				accountId,
				gw2AccountId,
			)
		}

		for _, method := range []string{http.MethodPut, http.MethodPatch} {
			t.Run(method, func(t *testing.T) {
				for _, path := range []string{"/", "/" + url.PathEscape(gw2AccountId.String())} {
					t.Run(path, func(t *testing.T) {
						t.Cleanup(func() {
							if !assert.NoError(t, truncateTablesFn()) {
								t.FailNow()
							}
						})

						testFn(t, method, path)
					})
				}
			})
		}

		return nil
	}))
}

func prepareMuxForAccountRequest(mux *http.ServeMux, token string, gw2AccountId uuid.UUID, name string) {
	mux.HandleFunc("/v2/account", func(w http.ResponseWriter, req *http.Request) {
		if strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(gw2.Account{
			Id:   gw2AccountId,
			Name: name,
		})
	})
}

func prepareMuxForTokenInfoRequest(mux *http.ServeMux, token, name string, perms ...gw2.Permission) {
	mux.HandleFunc("/v2/tokeninfo", func(w http.ResponseWriter, req *http.Request) {
		if strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(gw2.TokenInfo{
			Name:        name,
			Permissions: perms,
		})
	})
}

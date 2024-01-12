package test

import (
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type Fn func(t *testing.T, pool *pgxpool.Pool, conv *service.SessionJwtConverter, truncateTablesFn func() error)

func MustUnauthorized(t testing.TB, h http.Handler, req *http.Request) bool {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	return assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func RunAll(t *testing.T, scope *Scope, tests map[string]map[string]Fn) {
	conv := NewRandomConverter(t)
	assert.NoError(t, scope.WithPgx(func(pool *pgxpool.Pool, truncateTablesFn func() error) error {
		for name1, ht := range tests {
			t.Run(name1, func(t *testing.T) {
				for name2, fn := range ht {
					t.Run(name2, func(t *testing.T) {
						t.Cleanup(func() {
							if !assert.NoError(t, truncateTablesFn()) {
								t.FailNow()
							}
						})

						fn(t, pool, conv, truncateTablesFn)
					})
				}
			})
		}

		return nil
	}))
}

func Authenticated(t testing.TB, req *http.Request, pool *pgxpool.Pool, conv *service.SessionJwtConverter, issuer, idAtIssuer string) (uuid.UUID, string, time.Time, time.Time) {
	creationTime := time.Now()
	expirationTime := creationTime.Add(time.Hour)
	accountId, sessionId := NewUUID(t), NewUUID(t).String()

	k, err := service.NewKeyAndIv()
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	b, err := k.Encrypt([]byte(`{"lat": 52.5162778, "lng": 13.3755154}`))
	if !assert.NoError(t, err) {
		t.FailNow()
	}

	CreateAccountAndFederation(t, pool, accountId, issuer, idAtIssuer, creationTime)
	CreateSession(t, pool, sessionId, issuer, idAtIssuer, creationTime, expirationTime, b)

	cookieV, err := conv.WriteJWT(
		service.SessionJwtClaims{
			SessionId:     sessionId,
			EncryptionKey: k.ToBytes(),
		},
		expirationTime,
	)

	if !assert.NoError(t, err) {
		t.FailNow()
	}

	req.AddCookie(&http.Cookie{
		Name:  "BEARER",
		Value: cookieV,
	})
	req.Header.Set("Cloudfront-Viewer-Latitude", "52.5162778")
	req.Header.Set("Cloudfront-Viewer-Longitude", "13.3755154")

	return accountId, sessionId, creationTime, expirationTime
}

func AddQuery(req *http.Request, query ...string) {
	if len(query)%2 != 0 {
		panic("query must be pairs")
	}

	q := req.URL.Query()
	for i := 0; i < len(query); i += 2 {
		q.Add(query[i], query[i+1])
	}

	req.URL.RawQuery = q.Encode()
}

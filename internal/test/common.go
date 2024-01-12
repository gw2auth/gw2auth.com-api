package test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func NewRandomConverter(t testing.TB) *service.SessionJwtConverter {
	kid, priv, pub, err := newRandomKeys()
	if !assert.NoError(t, err) {
		t.FailNow()
		return nil
	}

	return service.NewSessionJwtConverter(kid, priv, map[string]*rsa.PublicKey{kid: pub})
}

func newRandomKeys() (string, *rsa.PrivateKey, *rsa.PublicKey, error) {
	kid := make([]byte, 32)
	_, err := rand.Read(kid)
	if err != nil {
		return "", nil, nil, err
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", nil, nil, err
	}

	return base64.RawStdEncoding.EncodeToString(kid), priv, priv.Public().(*rsa.PublicKey), nil
}

func NewGw2ApiClient(handler http.Handler) (*httptest.Server, *gw2.ApiClient) {
	s := httptest.NewServer(handler)
	gw2ApiClient := gw2.NewApiClient(s.Client(), s.URL)

	return s, gw2ApiClient
}

func WithGw2ApiClient(handler http.Handler, fn func(gw2ApiClient *gw2.ApiClient) error) error {
	s, gw2ApiClient := NewGw2ApiClient(handler)
	defer s.Close()

	return fn(gw2ApiClient)
}

func NewUUID(t testing.TB) uuid.UUID {
	v, err := uuid.NewV4()
	if assert.NoError(t, err) {
		return v
	}

	panic("invalid state")
}

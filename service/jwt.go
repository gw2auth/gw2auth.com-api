package service

import (
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

const issuer = "login.gw2auth.com"
const sessionClaim = "session"
const encryptionKeyClaim = "k"

type sessionJwtClaims struct {
	Session string `json:"session"`
	K       string `json:"k"`
	jwt.RegisteredClaims
}

type SessionJwtClaims struct {
	SessionId     string
	EncryptionKey []byte
}

type SessionJwtConverter struct {
	kid    string
	priv   *rsa.PrivateKey
	pub    map[string]*rsa.PublicKey
	parser *jwt.Parser
}

func NewSessionJwtConverter(kid string, priv *rsa.PrivateKey, pub map[string]*rsa.PublicKey) *SessionJwtConverter {
	return &SessionJwtConverter{
		kid:  kid,
		priv: priv,
		pub:  pub,
		parser: jwt.NewParser(
			jwt.WithIssuer(issuer),
			jwt.WithLeeway(time.Second*5),
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		),
	}
}

func (c *SessionJwtConverter) ReadJWT(jwtStr string) (SessionJwtClaims, time.Time, error) {
	tk, err := c.parser.Parse(jwtStr, func(tk *jwt.Token) (interface{}, error) {
		if m, ok := tk.Method.(*jwt.SigningMethodRSA); !ok || m != jwt.SigningMethodRS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", tk.Header["alg"])
		}

		claims, ok := tk.Claims.(jwt.MapClaims)
		if !ok {
			return nil, errors.New("expected MapClaims")
		}

		session, ok := claims[sessionClaim]
		if !ok {
			return nil, errors.New("expected session claim")
		} else if session, ok = session.(string); !ok || session == "" {
			return nil, errors.New("expected session claim to be a non-empty string")
		}

		k, ok := claims[encryptionKeyClaim]
		if !ok {
			return nil, errors.New("expected k claim")
		} else if k, ok = k.(string); !ok || k == "" {
			return nil, errors.New("expected k claim to be a non-empty string")
		}

		kid, ok := tk.Header["kid"].(string)
		if !ok {
			return nil, errors.New("kid header not found")
		}

		pub, ok := c.pub[kid]
		if !ok {
			return nil, errors.New("unknown kid")
		}

		return pub, nil
	})

	if err != nil {
		return SessionJwtClaims{}, time.Time{}, fmt.Errorf("failed to parse jwt: %w", err)
	}

	claims := tk.Claims.(jwt.MapClaims)
	k, err := base64.RawStdEncoding.DecodeString(claims[encryptionKeyClaim].(string))
	if err != nil {
		return SessionJwtClaims{}, time.Time{}, fmt.Errorf("k claim could not be decoded: %w", err)
	}

	iat, err := tk.Claims.GetIssuedAt()
	if err != nil {
		return SessionJwtClaims{}, time.Time{}, fmt.Errorf("iat claim could not be read: %w", err)
	}

	return SessionJwtClaims{
		SessionId:     claims[sessionClaim].(string),
		EncryptionKey: k,
	}, iat.Time, nil
}

func (c *SessionJwtConverter) WriteJWT(claims SessionJwtClaims, exp time.Time) (string, error) {
	now := time.Now()
	tk := jwt.NewWithClaims(jwt.SigningMethodRS256, sessionJwtClaims{
		Session: claims.SessionId,
		K:       base64.RawStdEncoding.EncodeToString(claims.EncryptionKey),
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        "",
			Issuer:    issuer,
			Subject:   "",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	})
	tk.Header["kid"] = c.kid

	return tk.SignedString(c.priv)
}

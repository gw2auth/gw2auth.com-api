package service

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"github.com/golang-jwt/jwt/v5"
	"strings"
	"testing"
	"time"
)

const kid = "77824899-854d-4836-9c9b-889fec5759d7"
const pemPub = `-----BEGIN X.509-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEApKg/9Qzt/80+Ym3pDfAV
3Uyh4BNLSU87zvUCpz9If43EL9FIQO3G7uXGki/sanT5D8u5RSJuHb+1PLnV9M1x
tTrwCeORY4APxw/Hyue8B4ZhXwk+uXrAhfx2x25vX/m6pshmhfVZ9t38ZAW2HY5I
G2OboU4DCOqDeQR8/NZB7iHfapmYdxcRSBn6f4IarY4eXSAb2s60sT9iuOvvxnA/
7/m7eZvGIzHERzqFl9OJdcoqsqzEmsKjXg047g/PZX2Soe5WxKYEKxlNFsy6LcII
4FTBAOEVvags8lyS4XMl8HDU5dg8z2sx9Df2rB6HGePuNdVSdnZ+yzlrAXELvJ8d
3QIDAQAB
-----END X.509-----`
const pemPriv = `-----BEGIN PKCS#8-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQCkqD/1DO3/zT5i
bekN8BXdTKHgE0tJTzvO9QKnP0h/jcQv0UhA7cbu5caSL+xqdPkPy7lFIm4dv7U8
udX0zXG1OvAJ45FjgA/HD8fK57wHhmFfCT65esCF/HbHbm9f+bqmyGaF9Vn23fxk
BbYdjkgbY5uhTgMI6oN5BHz81kHuId9qmZh3FxFIGfp/ghqtjh5dIBvazrSxP2K4
6+/GcD/v+bt5m8YjMcRHOoWX04l1yiqyrMSawqNeDTjuD89lfZKh7lbEpgQrGU0W
zLotwgjgVMEA4RW9qCzyXJLhcyXwcNTl2DzPazH0N/asHocZ4+411VJ2dn7LOWsB
cQu8nx3dAgMBAAECggEALfQhCXCm2b3727emE/Q9/em9wc5QtDCPVhauO2QfhS3Z
I6lKe4iA/TqWnUUPu7RSsHRgjClsRxQybjTFQFG4GubMiE4BTR44CQvf7EKZiRYZ
hc8MOPLH2X0Y31b0cXo+e/6xElDj31Cb+IRZi80iVbaKgE3H7gyZTcSRZ3UaqG1L
XXOpE6JZziLWNs0ieYoDWy/qtf5kKk4gZBv2BWvBWKW6NZUTfL4QFw2JpJRl7P0B
MuZl8IV1CWJCZwo7DltqgdXdC/bkTjOl+mbzT2KEb4jvATK42YhhiF0ge04Afy9R
bN230152HL5ahBCehP/h8Gms2RAAKoHaDKHX4RTRlwKBgQC3iWgTbteVPwdGwVs1
YNWkNc8E2HoPyXGEFmqAb15eF8qtigBbKlsH2c0n9RzbLaoIjbTkGpLyUaqUIbBR
G4HAZ1C4z139Xrd7P9C5GKpYWMnZQ2dJH2xtntkU5utNUh6znRADEio35K93ieQ2
airCX0otEFpCT7GulUX4XWgY/wKBgQDlqqKm0n7+ZcMF6AYrHGmUzoqKDaGZzbte
6jfIzKKdVXFn9MD2SX++gRjm8ezf4R0LaKANXDBs85DQS3U4piSYLPbNpy7CZ2nG
KoePCRmfoplu0SMwdRhM/JnlOTwGFwVUoJYNkMwkhASnjJyv79PydLi26ltXsPmY
bmGVkSFNIwKBgD2Xbga/drdODHoRHzOdiHRv3kYpA2VS27ZQ83KgbRG0eD6ehhoR
77SFwdQ77HAdNedi7qZxyIhrIYxOdeXyDivsP+mVJVyGsZo5wyiqf1fgi/ROK1Yd
pnxvBzh9ec9b1JPADISLTwGsy7mY263rGOhbo//VcgS4y87jpzR+3BUvAoGAMS/+
beQfNrTss9MhnxISusARg8evmJUrUASxtdu96BxokW8l9JmBNnfHsY6WwMwhwFPE
E5hu9qajmTjP/jX3GHBo21q29QPY74wkREoapsnYOpeoBbTOY46mFyXO6S79AUbz
XCxqzFdJ9+hxlmyy4/aDqQlATuOkUTUyySmwDCkCgYA2nMH9EcyqQEx+P8g77NtB
v+LqCi/qbnYr8Cw+CeLebQZ8iXgzxAbT73YW7c8MoOkc2nw/oybSEh3mSr+4CTux
KWS8Ns0lJQCcD11PqytC/Awc4DyxEAbBgksaDGiYE7P2LZy07Wb3W2VWSaOzvczK
9m1gyR6v3Cc9Lo5B4nNIog==
-----END PKCS#8-----`

func newConverter(t *testing.T) *SessionJwtConverter {
	pub, err := jwt.ParseRSAPublicKeyFromPEM([]byte(pemPub))
	if err != nil {
		t.Fatalf("failed to load pub: %v", err)
	}

	priv, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(pemPriv))
	if err != nil {
		t.Fatalf("failed to load priv: %v", err)
	}

	return NewSessionJwtConverter(kid, priv, map[string]*rsa.PublicKey{kid: pub})
}

func newRandomConverter(t *testing.T) *SessionJwtConverter {
	kid, priv, pub := newRandomKeys(t)
	return NewSessionJwtConverter(kid, priv, map[string]*rsa.PublicKey{kid: pub})
}

func newRandomKeys(t *testing.T) (string, *rsa.PrivateKey, *rsa.PublicKey) {
	kid := make([]byte, 32)
	_, err := rand.Read(kid)
	if err != nil {
		t.Fatalf("failed to generate kid: %v", err)
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate priv: %v", err)
	}

	return base64.RawStdEncoding.EncodeToString(kid), priv, priv.Public().(*rsa.PublicKey)
}

func TestSessionJwtConverter_WriteRead(t *testing.T) {
	conv := newConverter(t)
	testReadWrite(t, conv, conv)
}

func TestSessionJwtConverter_WriteReadExpired(t *testing.T) {
	conv := newConverter(t)
	jwtStr, err := conv.WriteJWT(SessionJwtClaims{
		SessionId:     "test",
		EncryptionKey: []byte{1, 2, 3, 4, 5},
	}, time.Now().Add(-time.Minute))

	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	_, _, err = conv.ReadJWT(jwtStr)
	if err == nil {
		t.Fatal("should fail to read expired jwt; err is nil")
	}
}

func TestSessionJwtConverter_WriteReadInvalidSignature1(t *testing.T) {
	jwtStr, err := newConverter(t).WriteJWT(SessionJwtClaims{
		SessionId:     "test",
		EncryptionKey: []byte{1, 2, 3, 4, 5},
	}, time.Now().Add(time.Minute))

	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	_, _, err = newRandomConverter(t).ReadJWT(jwtStr)
	if err == nil {
		t.Fatal("should fail to read with invalid signature; err is nil")
	}
}

func TestSessionJwtConverter_WriteReadInvalidSignature2(t *testing.T) {
	convA := newConverter(t)
	convB := newRandomConverter(t)

	claims := SessionJwtClaims{
		SessionId:     "test",
		EncryptionKey: []byte{1, 2, 3, 4, 5},
	}
	exp := time.Now().Add(time.Minute)

	jwtStrA, err := convA.WriteJWT(claims, exp)
	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	jwtStrB, err := convB.WriteJWT(claims, exp)
	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	partsA := strings.Split(jwtStrA, ".")
	partsB := strings.Split(jwtStrB, ".")
	partsA[2] = partsB[2]

	_, _, err = convA.ReadJWT(strings.Join(partsA, "."))
	if err == nil {
		t.Fatal("should fail to read with invalid signature; err is nil")
	}
}

func TestSessionJwtConverter_WriteReadInvalidSignature3(t *testing.T) {
	conv := newConverter(t)

	exp := time.Now().Add(time.Minute)
	jwtStrA, err := conv.WriteJWT(SessionJwtClaims{
		SessionId:     "test",
		EncryptionKey: []byte{1, 2, 3, 4, 5},
	}, exp)

	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	jwtStrB, err := conv.WriteJWT(SessionJwtClaims{
		SessionId:     "test2",
		EncryptionKey: []byte{1, 2, 3, 4, 5},
	}, exp)

	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	partsA := strings.Split(jwtStrA, ".")
	partsB := strings.Split(jwtStrB, ".")
	partsA[1] = partsB[1]

	_, _, err = conv.ReadJWT(strings.Join(partsA, "."))
	if err == nil {
		t.Fatal("should fail to read with invalid signature; err is nil")
	}
}

func TestSessionJwtConverter_WriteReadWithTwoKnownPubs(t *testing.T) {
	kidA, privA, pubA := newRandomKeys(t)
	kidB, privB, pubB := newRandomKeys(t)

	pub := map[string]*rsa.PublicKey{
		kidA: pubA,
		kidB: pubB,
	}

	convA := NewSessionJwtConverter(kidA, privA, pub)
	convB := NewSessionJwtConverter(kidB, privB, pub)

	matrix := [][2]*SessionJwtConverter{
		{convA, convA},
		{convB, convB},
		{convA, convB},
		{convB, convA},
	}
	matrixNames := []string{
		"write A, read A",
		"write B, read B",
		"write A, read B",
		"write B, read A",
	}

	for i, v := range matrix {
		t.Run(matrixNames[i], func(t *testing.T) {
			testReadWrite(t, v[0], v[1])
		})
	}
}

func testReadWrite(t *testing.T, convA *SessionJwtConverter, convB *SessionJwtConverter) {
	start := time.Now().Truncate(time.Second)
	exp := start.Add(time.Minute)
	jwtStr, err := convA.WriteJWT(SessionJwtClaims{
		SessionId:     "test",
		EncryptionKey: []byte{1, 2, 3, 4, 5},
	}, exp)
	end := time.Now().Truncate(time.Second)

	if err != nil {
		t.Fatalf("failed to write jwt: %v", err)
	}

	claims, iat, err := convB.ReadJWT(jwtStr)
	if err != nil {
		t.Fatalf("failed to read jwt: %v", err)
	}

	if claims.SessionId != "test" {
		t.Fatalf("invalid session id: %v", claims.SessionId)
	}

	if !bytes.Equal(claims.EncryptionKey, []byte{1, 2, 3, 4, 5}) {
		t.Fatalf("invalid encryption key: %v", claims.EncryptionKey)
	}

	if iat.Before(start) || iat.After(end) {
		t.Fatalf("iat out of bounds: expected [start(%v) < iat(%v) < end(%v)]", start, iat, end)
	}
}

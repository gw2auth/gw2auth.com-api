package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gw2auth/gw2auth.com-api/util"
	"golang.org/x/crypto/argon2"
	"strconv"
	"strings"
)

var (
	defaultArgon2IdKey = Argon2IdKey{}
)

type Argon2Params struct {
	version int
	salt    []byte
	time    uint32 // =iterations
	memory  uint32 // KiB
	threads uint8  // =parallelism
	keyLen  uint32
}

func (p Argon2Params) IDKey(secret []byte) ([]byte, error) {
	if argon2.Version != p.version {
		return nil, fmt.Errorf("can not compute argon2id hash for version %d (library version=%d)", p.version, argon2.Version)
	}

	return argon2.IDKey(secret, p.salt, p.time, p.memory, p.threads, p.keyLen), nil
}

type Argon2IdKey struct {
	p    Argon2Params
	hash []byte
}

func NewArgon2IdKeyFromSecret(secret []byte) (Argon2IdKey, error) {
	// Argon2PasswordEncoder defaultsForSpringSecurity_v5_8
	const saltLength, hashLength, parallelism, memoryKB, iterations = 16, 32, 1, 16384, 2

	salt, err := GenerateRandomBytes(saltLength)
	if err != nil {
		return defaultArgon2IdKey, err
	}

	k := Argon2IdKey{
		p: Argon2Params{
			version: argon2.Version,
			salt:    salt,
			time:    iterations,
			memory:  memoryKB,
			threads: parallelism,
			keyLen:  hashLength,
		},
	}

	if k.hash, err = k.p.IDKey(secret); err != nil {
		return defaultArgon2IdKey, err
	}

	return k, nil
}

func NewArgon2IdKeyFromString(encoded string) (Argon2IdKey, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return defaultArgon2IdKey, errors.New("expected 5 encoded parts (type, version, params, salt, hash)")
	}

	parts = parts[1:]
	t, v, p, s, h := parts[0], parts[1], parts[2], parts[3], parts[4]
	if t != "argon2id" {
		return defaultArgon2IdKey, errors.New("expected type argon2id")
	}

	if !strings.HasPrefix(v, "v=") {
		return defaultArgon2IdKey, errors.New("version (2nd part) must start with [v=]")
	}

	var k Argon2IdKey
	var err error

	if k.p.version, err = strconv.Atoi(strings.TrimPrefix(v, "v=")); err != nil {
		return defaultArgon2IdKey, err
	}

	// params
	{
		for _, part := range strings.Split(p, ",") {
			if key, val, ok := strings.Cut(part, "="); ok {
				switch key {
				case "m":
					k.p.memory, err = util.ParseUint32(val)

				case "t":
					k.p.time, err = util.ParseUint32(val)

				case "p":
					k.p.threads, err = util.ParseUint8(val)
				}

				if err != nil {
					return defaultArgon2IdKey, err
				}
			}
		}

		if k.p.memory == 0 || k.p.time == 0 || k.p.threads == 0 {
			return defaultArgon2IdKey, errors.New("missing required param (3rd part); expected [m,t,p]")
		}
	}

	if k.p.salt, err = base64.RawStdEncoding.DecodeString(s); err != nil {
		return defaultArgon2IdKey, err
	}

	if k.hash, err = base64.RawStdEncoding.DecodeString(h); err != nil {
		return defaultArgon2IdKey, err
	}

	k.p.keyLen = uint32(len(k.hash))

	return k, nil
}

func (k Argon2IdKey) String() string {
	// compatible with spring Argon2PasswordEncoder
	r := "$argon2id"
	r += fmt.Sprintf("$v=%d$m=%d,t=%d,p=%d", k.p.version, k.p.memory, k.p.time, k.p.threads)
	// Base64.getEncoder().withoutPadding()
	r += "$" + base64.RawStdEncoding.EncodeToString(k.p.salt)
	r += "$" + base64.RawStdEncoding.EncodeToString(k.hash)

	return r
}

func (k Argon2IdKey) Verify(secret []byte) bool {
	hash, err := k.p.IDKey(secret)
	if err != nil {
		return false
	}

	return bytes.Equal(k.hash, hash)
}

func EncodeArgon2id(secret []byte) (string, error) {
	k, err := NewArgon2IdKeyFromSecret(secret)
	if err != nil {
		return "", err
	}

	return k.String(), err
}

func VerifyArgon2id(encoded string, secret []byte) bool {
	k, err := NewArgon2IdKeyFromString(encoded)
	if err != nil {
		return false
	}

	return k.Verify(secret)
}

func GenerateRandomBytes(len int) ([]byte, error) {
	b := make([]byte, len)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}

	return b, nil
}

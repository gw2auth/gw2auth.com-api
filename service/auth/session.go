package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"math"
	"time"
)

type Session struct {
	Id                  string
	AccountId           uuid.UUID
	AccountCreationTime time.Time
	Issuer              string
	IdAtIssuer          string
	CreationTime        time.Time
	ExpirationTime      time.Time
	Metadata            SessionMetadata
}

type SessionMetadata struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type PgxConn interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func LoadAndUpdateSession(ctx context.Context, conn PgxConn, id string, encryptionKey []byte, issuedAt time.Time, newMetadata SessionMetadata, sess *Session) error {
	sql := `
SELECT
	acc.id,
	acc.creation_time,
	acc_fed.issuer,
	acc_fed.id_at_issuer,
	acc_fed_sess.creation_time,
	acc_fed_sess.expiration_time,
	acc_fed_sess.metadata
FROM account_federation_sessions acc_fed_sess
INNER JOIN account_federations acc_fed
ON acc_fed_sess.issuer = acc_fed.issuer AND acc_fed_sess.id_at_issuer = acc_fed.id_at_issuer
INNER JOIN accounts acc
ON acc_fed.account_id = acc.id
WHERE acc_fed_sess.id = $1
FOR UPDATE OF acc_fed_sess
`

	var rawMetadata []byte
	err := conn.QueryRow(ctx, sql, id).Scan(
		&sess.AccountId,
		&sess.AccountCreationTime,
		&sess.Issuer,
		&sess.IdAtIssuer,
		&sess.CreationTime,
		&sess.ExpirationTime,
		&rawMetadata,
	)

	if err != nil {
		return fmt.Errorf("failed to load session: %w", err)
	}

	k, err := service.NewKeyAndIvFromBytes(encryptionKey)
	if err != nil {
		return fmt.Errorf("could not load encryption key from bytes: %w", err)
	}

	rawMetadata, err = k.Decrypt(rawMetadata)
	if err != nil {
		return fmt.Errorf("failed to decrypt stored metadata: %w", err)
	}

	if err = json.Unmarshal(rawMetadata, &sess.Metadata); err != nil {
		return fmt.Errorf("failed to parse stored metadata: %w", err)
	}

	if !isMetadataPlausible(sess.Metadata, newMetadata, time.Now().Sub(issuedAt)) {
		return errors.New("metadata not plausible")
	}

	if rawMetadata, err = json.Marshal(newMetadata); err != nil {
		return fmt.Errorf("failed to marshal new metadata: %w", err)
	}

	if rawMetadata, err = k.Encrypt(rawMetadata); err != nil {
		return fmt.Errorf("failed to encrypt new metadata: %w", err)
	}

	sql = `
UPDATE account_federation_sessions
SET expiration_time = $2, metadata = $3
WHERE id = $1
`
	newExpTime := time.Now().Add(time.Hour * 24 * 30)
	_, err = conn.Exec(ctx, sql, id, newExpTime, rawMetadata)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	sess.Id = id
	sess.ExpirationTime = newExpTime
	sess.Metadata = newMetadata

	return nil
}

func DeleteSession(ctx context.Context, conn PgxConn, id string) error {
	_, err := conn.Exec(ctx, "DELETE FROM account_federation_sessions WHERE id = $1", id)
	return err
}

func isMetadataPlausible(orig SessionMetadata, current SessionMetadata, passed time.Duration) bool {
	travelledKm := distance(orig.Lat, orig.Lng, current.Lat, current.Lng)
	if travelledKm > 1000 {
		// never allow to travel more than 1000km
		return false
	}

	if travelledKm <= 30 {
		// always allow to travel 30km
		return true
	}

	return travelledKm <= (333.3 * (passed.Seconds() / 86400.0))
}

// https://gist.github.com/hotdang-ca/6c1ee75c48e515aec5bc6db6e3265e49
// value returned is distance in kilometers
func distance(lat1 float64, lng1 float64, lat2 float64, lng2 float64) float64 {
	radlat1 := float64(math.Pi * lat1 / 180)
	radlat2 := float64(math.Pi * lat2 / 180)

	theta := float64(lng1 - lng2)
	radtheta := float64(math.Pi * theta / 180)

	dist := math.Sin(radlat1)*math.Sin(radlat2) + math.Cos(radlat1)*math.Cos(radlat2)*math.Cos(radtheta)
	if dist > 1 {
		dist = 1
	}

	dist = math.Acos(dist)
	dist = dist * 180 / math.Pi
	dist = dist * 60 * 1.1515
	dist = dist * 1.609344

	return dist
}

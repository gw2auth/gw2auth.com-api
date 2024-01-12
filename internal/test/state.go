package test

import (
	"github.com/gofrs/uuid/v5"
	"github.com/gw2auth/gw2auth.com-api/service/gw2"
	"github.com/jackc/pgx/v5/pgxpool"
	"testing"
	"time"
)

func CreateAccount(t testing.TB, pool *pgxpool.Pool, id uuid.UUID, creationTime time.Time) {
	MustExec(t, pool, `INSERT INTO accounts (id, creation_time) VALUES ($1, $2)`, id, creationTime)
}

func CreateAccountFederation(t testing.TB, pool *pgxpool.Pool, accountId uuid.UUID, issuer, idAtIssuer string) {
	MustExec(
		t,
		pool,
		`INSERT INTO account_federations (issuer, id_at_issuer, account_id) VALUES ($1, $2, $3)`,
		issuer,
		idAtIssuer,
		accountId,
	)
}

func CreateAccountAndFederation(t testing.TB, pool *pgxpool.Pool, accountId uuid.UUID, issuer, idAtIssuer string, creationTime time.Time) {
	CreateAccount(t, pool, accountId, creationTime)
	CreateAccountFederation(t, pool, accountId, issuer, idAtIssuer)
}

func CreateSession(t testing.TB, pool *pgxpool.Pool, sessionId, issuer, idAtIssuer string, creationTime, expirationTime time.Time, metadata []byte) {
	MustExec(
		t,
		pool,
		`
INSERT INTO account_federation_sessions
(id, issuer, id_at_issuer, creation_time, expiration_time, metadata)
VALUES
($1, $2, $3, $4, $5, $6)
`,
		sessionId,
		issuer,
		idAtIssuer,
		creationTime,
		expirationTime,
		metadata,
	)
}

func CreateGw2Account(t testing.TB, pool *pgxpool.Pool, accountId, gw2AccountId uuid.UUID, name, displayName string) {
	now := time.Now()
	MustExec(
		t,
		pool,
		`
INSERT INTO gw2_accounts
(account_id, gw2_account_id, creation_time, display_name, order_rank, gw2_account_name, last_name_check_time)
VALUES
($1, $2, $3, $4, $5, $6, $7)
`,
		accountId,
		gw2AccountId,
		now,
		displayName,
		"A",
		name,
		now,
	)
}

func CreateGw2ApiToken(t testing.TB, pool *pgxpool.Pool, accountId, gw2AccountId uuid.UUID, token string, perms []gw2.Permission) {
	now := time.Now()
	MustExec(
		t,
		pool,
		`
INSERT INTO gw2_account_api_tokens
(account_id, gw2_account_id, creation_time, gw2_api_token, gw2_api_permissions_bit_set, last_valid_time, last_valid_check_time)
VALUES
($1, $2, $3, $4, $5, $6, $7)
`,
		accountId,
		gw2AccountId,
		now,
		token,
		gw2.PermissionsToBitSet(perms),
		now,
		now,
	)
}

func CreateGw2AccountVerification(t testing.TB, pool *pgxpool.Pool, accountId, gw2AccountId uuid.UUID) {
	MustExec(
		t,
		pool,
		`
INSERT INTO gw2_account_verifications
(gw2_account_id, account_id)
VALUES ($1, $2)
`,
		gw2AccountId,
		accountId,
	)
}

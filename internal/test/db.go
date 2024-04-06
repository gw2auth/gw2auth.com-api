package test

import (
	"cmp"
	"context"
	"crypto/rand"
	"embed"
	"encoding/binary"
	"errors"
	"fmt"
	crdbpgx "github.com/cockroachdb/cockroach-go/v2/crdb/crdbpgxv5"
	"github.com/cockroachdb/cockroach-go/v2/testserver"
	pgxuuid "github.com/jackc/pgx-gofrs-uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	appUser       = "gw2auth_app"
	migrationUser = "flyway"
)

type Scope struct {
	ts    atomic.Pointer[testserver.TestServer]
	mutex sync.Mutex
}

func (s *Scope) getOrCreateConfig() (*pgxpool.Config, error) {
	var config *pgxpool.Config

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if pTs := s.ts.Load(); pTs == nil {
		ts, err := testserver.NewTestServer(testserver.CustomVersionOpt("v23.2.3"))
		if err != nil {
			return nil, err
		}

		if config, err = pgxpool.ParseConfig(ts.PGURL().String()); err != nil {
			ts.Stop()
			return nil, err
		}

		if err = createUsers(config); err != nil {
			ts.Stop()
			return nil, err
		}

		if !s.ts.CompareAndSwap(nil, &ts) {
			ts.Stop()
			return nil, errors.New("race starting testserver")
		}
	} else {
		var err error
		if config, err = pgxpool.ParseConfig((*pTs).PGURL().String()); err != nil {
			return nil, err
		}
	}

	return config, nil
}

func (s *Scope) WithPgx(fn func(pool *pgxpool.Pool, truncateTablesFn func() error) error) error {
	config, err := s.getOrCreateConfig()
	if err != nil {
		return err
	}

	return withNewDatabase(config, func(config *pgxpool.Config) error {
		var err error
		if err = initDatabase(config); err != nil {
			return err
		}

		poolMigration, err := connect(config, migrationUser)
		if err != nil {
			return err
		}

		pool, err := connect(config, appUser)
		if err != nil {
			return err
		}

		defer pool.Close()
		defer poolMigration.Close()

		return fn(pool, func() error {
			return truncateTables(poolMigration)
		})
	})
}

func (s *Scope) Stop() {
	if ts := s.ts.Swap(nil); ts != nil {
		(*ts).Stop()
	}
}

func WithScope(fn func(scope *Scope)) {
	s := new(Scope)
	defer s.Stop()
	fn(s)
}

func MustExec(t testing.TB, pool *pgxpool.Pool, sql string, args ...any) {
	_, err := pool.Exec(context.Background(), sql, args...)
	assert.NoError(t, err)
}

func MustExist(t testing.TB, pool *pgxpool.Pool, sql string, args ...any) bool {
	var b bool
	err := pool.QueryRow(context.Background(), sql, args...).Scan(&b)
	return assert.NoError(t, err) && b
}

func MustNotExist(t testing.TB, pool *pgxpool.Pool, sql string, args ...any) bool {
	var b bool
	err := pool.QueryRow(context.Background(), sql, args...).Scan(&b)
	return assert.ErrorIs(t, err, pgx.ErrNoRows) && !b
}

func truncateTables(pool *pgxpool.Pool) error {
	ctx := context.Background()
	return crdbpgx.ExecuteTx(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname = $1", "public")
		if err != nil {
			return err
		}

		tableNames, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
			var tableName string
			return tableName, row.Scan(&tableName)
		})

		if err != nil {
			return err
		}

		for _, tableName := range tableNames {
			if _, err = tx.Exec(ctx, fmt.Sprintf(`TRUNCATE TABLE "public"."%s" CASCADE`, tableName)); err != nil {
				return err
			}
		}

		return nil
	})
}

func withNewDatabase(config *pgxpool.Config, fn func(config *pgxpool.Config) error) error {
	dbName, err := generateDbName()
	if err != nil {
		return err
	}

	poolInit, err := connect(config, config.ConnConfig.User)
	if err != nil {
		return err
	}

	defer poolInit.Close()

	if _, err = poolInit.Exec(context.Background(), "CREATE DATABASE "+dbName); err != nil {
		return err
	}

	defer func() {
		_, _ = poolInit.Exec(context.Background(), "DROP DATABASE "+dbName)
	}()

	config = config.Copy()
	config.ConnConfig.Database = dbName

	return fn(config)
}

func createUsers(config *pgxpool.Config) error {
	poolInit, err := connect(config, config.ConnConfig.User)
	if err != nil {
		return err
	}

	defer poolInit.Close()

	_, err = poolInit.Exec(context.Background(), "CREATE USER "+appUser)
	if err != nil {
		return err
	}

	_, err = poolInit.Exec(context.Background(), "CREATE USER "+migrationUser)
	if err != nil {
		return err
	}

	return nil
}

func initDatabase(config *pgxpool.Config) error {
	poolMigration, err := connect(config, migrationUser)
	if err != nil {
		return err
	}

	defer poolMigration.Close()

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}

	slices.SortFunc(entries, func(a, b fs.DirEntry) int {
		vAStr, _, _ := strings.Cut(a.Name(), "_")
		vBStr, _, _ := strings.Cut(b.Name(), "_")
		vAStr, vBStr = vAStr[1:], vBStr[1:]

		vA, _ := strconv.Atoi(vAStr)
		vB, _ := strconv.Atoi(vBStr)

		return cmp.Compare(vA, vB)
	})

	for _, entry := range entries {
		var content string
		content, err = readMigrationFile(entry.Name())
		if err != nil {
			return err
		}

		err = crdbpgx.ExecuteTx(context.Background(), poolMigration, pgx.TxOptions{IsoLevel: pgx.ReadUncommitted}, func(tx pgx.Tx) error {
			// ugly but it works
			for _, sql := range strings.Split(content, ";") {
				_, err = poolMigration.Exec(context.Background(), sql)
				if err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func connect(config *pgxpool.Config, user string) (*pgxpool.Pool, error) {
	config = config.Copy()
	config.ConnConfig.User = user
	config.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxuuid.Register(conn.TypeMap())
		return nil
	}

	return pgxpool.NewWithConfig(context.Background(), config)
}

func readMigrationFile(name string) (string, error) {
	f, err := migrations.Open(fmt.Sprintf("migrations/%s", name))
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(b), err
}

func generateDbName() (string, error) {
	const length = 16
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"

	r := make([]byte, length)
	b := make([]byte, 4)

	for i := range r {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}

		r[i] = chars[binary.BigEndian.Uint32(b)%uint32(len(chars))]
	}

	return "test_" + string(r), nil
}

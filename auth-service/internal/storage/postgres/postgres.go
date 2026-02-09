package postgres

import (
	"context"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/byoverr/auth-service/internal/storage"
)

type Store struct {
	db *gorm.DB
	tu string
	ts string
}

func Connect(dsn string, schema string) (*Store, error) {
	// dsn пример:
	// "host=localhost user=auth password=authpass dbname=authdb port=5432 sslmode=disable"
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	_, err = gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("gorm db: %w", err)
	}
	// sqlDB.SetMaxOpenConns(20)
	// sqlDB.SetMaxIdleConns(10)
	// sqlDB.SetConnMaxLifetime(time.Hour)

	tu, ts := "users", "refresh_sessions"
	if schema != "" {
		tu = schema + "." + tu
		ts = schema + "." + ts
	}

	return &Store{db: gdb, tu: tu, ts: ts}, nil
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) Users() storage.UsersRepo {
	return &usersRepo{db: s.db, table: s.tu}
}

func (s *Store) Sessions() storage.SessionsRepo {
	return &sessionsRepo{db: s.db, table: s.ts}
}

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx storage.TxStore) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &txStore{db: tx, tu: s.tu, ts: s.ts})
	})
}

type txStore struct {
	db *gorm.DB
	tu string
	ts string
}

func (t *txStore) Users() storage.UsersRepo {
	return &usersRepo{db: t.db, table: t.tu}
}
func (t *txStore) Sessions() storage.SessionsRepo {
	return &sessionsRepo{db: t.db, table: t.ts}
}

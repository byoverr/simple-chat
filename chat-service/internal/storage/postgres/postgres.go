package postgres

import (
	"context"
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

type StoreInterface interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, tx TxStoreInterface) error) error
	Chats() ChatsRepo
	Messages() MessagesRepo
}

type TxStoreInterface interface {
	Chats() ChatsRepo
	Messages() MessagesRepo
}

func Connect(dsn string) (*Store, error) {
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(10)
	return &Store{db: gdb}, nil
}

func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) Chats() ChatsRepo {
	return &chatsRepo{db: s.db}
}

func (s *Store) Messages() MessagesRepo {
	return &messagesRepo{db: s.db}
}

func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, tx TxStoreInterface) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ctx, &txStore{db: tx})
	})
}

type txStore struct {
	db *gorm.DB
}

func (t *txStore) Chats() ChatsRepo       { return &chatsRepo{db: t.db} }
func (t *txStore) Messages() MessagesRepo { return &messagesRepo{db: t.db} }

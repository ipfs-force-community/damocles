package kvstore

import (
	"context"
	"fmt"

	"github.com/dgraph-io/badger/v2"
)

var _ KVStore = (*BadgerKVStore)(nil)

func DefaultBadgerOption(path string) badger.Options {
	return badger.DefaultOptions(path)
}

func OpenBadger(opt badger.Options) (*BadgerKVStore, error) {
	db, err := badger.Open(opt)
	if err != nil {
		return nil, err
	}

	return &BadgerKVStore{
		db: db,
	}, nil
}

type BadgerKVStore struct {
	db *badger.DB
}

func (b *BadgerKVStore) Get(ctx context.Context, key Key) (Val, error) {
	var val []byte
	err := b.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(key); err {
		case nil:
			val, err = item.ValueCopy(nil)
			return err

		case badger.ErrKeyNotFound:
			return ErrKeyNotFound

		default:
			return fmt.Errorf("get value from badger: %w", err)
		}
	})

	if err != nil {
		return nil, err
	}

	return val, nil
}

func (b *BadgerKVStore) Has(ctx context.Context, key Key) (bool, error) {
	err := b.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	switch err {
	case badger.ErrKeyNotFound:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, fmt.Errorf("failed to check if block exists in badger blockstore: %w", err)
	}
}

func (b *BadgerKVStore) View(ctx context.Context, key Key, cb Callback) error {
	return b.db.View(func(txn *badger.Txn) error {
		switch item, err := txn.Get(key); err {
		case nil:
			return item.Value(cb)

		case badger.ErrKeyNotFound:
			return ErrKeyNotFound

		default:
			return fmt.Errorf("get value from badger: %w", err)
		}
	})

}

func (b *BadgerKVStore) Put(ctx context.Context, key Key, val Val) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, val)
	})
}

func (b *BadgerKVStore) Del(ctx context.Context, key Key) error {
	return b.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (b *BadgerKVStore) Run(context.Context) error { return nil }

func (b *BadgerKVStore) Close(context.Context) error {
	return b.db.Close()
}

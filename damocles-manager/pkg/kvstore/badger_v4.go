package kvstore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/dgraph-io/badger/v4"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var _ KVStore = (*BadgerV4KVStore)(nil)

var bV4log = logging.New("kv").With("driver", "badger-v4")

type bV4logger struct {
	*logging.ZapLogger
}

func (bl *bV4logger) Warningf(format string, args ...interface{}) {
	bl.ZapLogger.Warnf(format, args...)
}

func OpenBadgerV4(basePath string) *BadgerV4DB {
	return &BadgerV4DB{
		basePath: basePath,
		dbs:      make(map[string]*badger.DB),
		mu:       sync.Mutex{},
	}
}

type BadgerV4KVStore struct {
	db *badger.DB
}

func (b *BadgerV4KVStore) View(ctx context.Context, f func(Txn) error) error {
	err := b.db.View(func(txn *badger.Txn) error {
		return f(&BadgerV4Txn{inner: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (b *BadgerV4KVStore) Update(ctx context.Context, f func(Txn) error) error {
	err := b.db.Update(func(txn *badger.Txn) error {
		return f(&BadgerV4Txn{inner: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (b *BadgerV4KVStore) NeedRetryTransactions() bool {
	return true
}

func (b *BadgerV4KVStore) Get(ctx context.Context, key Key) (val Val, err error) {
	for {
		err = b.View(ctx, func(txn Txn) error {
			v, err := txn.Get(key)
			if err != nil {
				return err
			}
			val = v
			return nil
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return
		}
	}
}

func (b *BadgerV4KVStore) Peek(ctx context.Context, key Key, f func(Val) error) error {
	for {
		err := b.View(ctx, func(txn Txn) error {
			return txn.Peek(key, f)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerV4KVStore) Put(ctx context.Context, key Key, val Val) error {
	for {
		err := b.Update(ctx, func(txn Txn) error {
			return txn.Put(key, val)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerV4KVStore) Del(ctx context.Context, key Key) error {
	for {
		err := b.Update(ctx, func(txn Txn) error {
			return txn.Del(key)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerV4KVStore) Scan(ctx context.Context, prefix Prefix) (it Iter, err error) {
	txn := b.db.NewTransaction(false)
	iter := txn.NewIterator(badger.DefaultIteratorOptions)

	return &BadgerV4IterWithoutTrans{
		BadgerV4Iter: &BadgerV4Iter{
			txn:    txn,
			iter:   iter,
			seeked: false,
			valid:  false,
			prefix: prefix,
		}}, nil
}

type BadgerV4IterWithoutTrans struct {
	*BadgerV4Iter
}

func (bi *BadgerV4IterWithoutTrans) Close() {
	bi.BadgerV4Iter.Close()
	bi.txn.Discard()
}

var _ Txn = (*BadgerV4Txn)(nil)

type BadgerV4Txn struct {
	inner *badger.Txn
}

func (txn *BadgerV4Txn) Get(key Key) (Val, error) {
	var val []byte

	switch item, err := txn.inner.Get(key); err {
	case nil:
		val, err = item.ValueCopy(nil)
		if err != nil {
			return val, err
		}
	case badger.ErrKeyNotFound:
		return val, ErrKeyNotFound

	default:
		return val, fmt.Errorf("get value from badger: %w", err)
	}

	return val, nil
}

func (txn *BadgerV4Txn) Peek(key Key, f func(Val) error) error {
	switch item, err := txn.inner.Get(key); err {
	case nil:
		return item.Value(f)

	case badger.ErrKeyNotFound:
		return ErrKeyNotFound

	default:
		return fmt.Errorf("get value from badger: %w", err)
	}

}

func (txn *BadgerV4Txn) Put(key Key, val Val) error {
	return txn.inner.Set(key, val)
}

func (txn *BadgerV4Txn) Del(key Key) error {
	return txn.inner.Delete(key)
}

func (txn *BadgerV4Txn) Scan(prefix Prefix) (Iter, error) {
	it := txn.inner.NewIterator(badger.DefaultIteratorOptions)

	return &BadgerV4Iter{
		txn:    txn.inner,
		iter:   it,
		seeked: false,
		valid:  false,
		prefix: prefix,
	}, nil
}

var _ Iter = (*BadgerV4Iter)(nil)

type BadgerV4Iter struct {
	txn  *badger.Txn
	iter *badger.Iterator
	item *badger.Item

	seeked bool
	valid  bool
	prefix []byte
}

func (bi *BadgerV4Iter) Next() bool {
	if bi.seeked {
		bi.iter.Next()
	} else {
		if len(bi.prefix) == 0 {
			bi.iter.Rewind()
		} else {
			bi.iter.Seek(bi.prefix)
		}
		bi.seeked = true
	}

	if len(bi.prefix) == 0 {
		bi.valid = bi.iter.Valid()
	} else {
		bi.valid = bi.iter.ValidForPrefix(bi.prefix)
	}

	if bi.valid {
		bi.item = bi.iter.Item()
	}

	return bi.valid
}

func (bi *BadgerV4Iter) Key() Key {
	if !bi.valid {
		return nil
	}

	return bi.item.Key()
}

func (bi *BadgerV4Iter) View(ctx context.Context, f func(Val) error) error {
	if !bi.valid {
		return ErrIterItemNotValid
	}

	return bi.item.Value(f)
}

func (bi *BadgerV4Iter) Close() {
	bi.iter.Close()
}

var _ DB = (*BadgerV4DB)(nil)

type BadgerV4DB struct {
	basePath string
	dbs      map[string]*badger.DB
	mu       sync.Mutex
}

func (db *BadgerV4DB) Run(context.Context) error {
	return nil
}

func (db *BadgerV4DB) Close(context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	var lastError error
	for k, innerDB := range db.dbs {
		if err := innerDB.Close(); err != nil {
			lastError = err
		}
		delete(db.dbs, k)
	}
	return lastError
}

func (db *BadgerV4DB) OpenCollection(_ context.Context, name string) (KVStore, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if innerDB, ok := db.dbs[name]; ok {
		return &BadgerV4KVStore{db: innerDB}, nil
	}
	path := filepath.Join(db.basePath, name)
	opts := badger.DefaultOptions(path).WithLogger(&bV4logger{bV4log.With("path", path)})
	innerDB, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open sub badger %s, %w", name, err)
	}
	db.dbs[name] = innerDB
	return &BadgerV4KVStore{db: innerDB}, nil
}

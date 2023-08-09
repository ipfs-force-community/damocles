package kvstore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/dgraph-io/badger/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var _ KVStore = (*BadgerV2KVStore)(nil)

var bV2log = logging.New("kv").With("driver", "badger-v2")

type bV2logger struct {
	*logging.ZapLogger
}

func (bl *bV2logger) Warningf(format string, args ...interface{}) {
	bl.ZapLogger.Warnf(format, args...)
}

func OpenBadgerV2(basePath string) *BadgerV2DB {
	return &BadgerV2DB{
		basePath: basePath,
		dbs:      make(map[string]*badger.DB),
		mu:       sync.Mutex{},
	}
}

type BadgerV2KVStore struct {
	db *badger.DB
}

func (b *BadgerV2KVStore) View(ctx context.Context, f func(Txn) error) error {
	err := b.db.View(func(txn *badger.Txn) error {
		return f(&BadgerV2Txn{inner: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (b *BadgerV2KVStore) Update(ctx context.Context, f func(Txn) error) error {
	err := b.db.Update(func(txn *badger.Txn) error {
		return f(&BadgerV2Txn{inner: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (b *BadgerV2KVStore) NeedRetryTransactions() bool {
	return true
}

func (b *BadgerV2KVStore) Get(ctx context.Context, key Key) (val Val, err error) {
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

func (b *BadgerV2KVStore) Peek(ctx context.Context, key Key, f func(Val) error) error {
	for {
		err := b.View(ctx, func(txn Txn) error {
			return txn.Peek(key, f)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerV2KVStore) Put(ctx context.Context, key Key, val Val) error {
	for {
		err := b.Update(ctx, func(txn Txn) error {
			return txn.Put(key, val)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerV2KVStore) Del(ctx context.Context, key Key) error {
	for {
		err := b.Update(ctx, func(txn Txn) error {
			return txn.Del(key)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerV2KVStore) Scan(ctx context.Context, prefix Prefix) (it Iter, err error) {
	txn := b.db.NewTransaction(false)
	iter := txn.NewIterator(badger.DefaultIteratorOptions)

	return &BadgerV2IterWithoutTrans{
		BadgerV2Iter: &BadgerV2Iter{
			txn:    txn,
			iter:   iter,
			seeked: false,
			valid:  false,
			prefix: prefix,
		}}, nil
}

type BadgerV2IterWithoutTrans struct {
	*BadgerV2Iter
}

func (bi *BadgerV2IterWithoutTrans) Close() {
	bi.BadgerV2Iter.Close()
	bi.txn.Discard()
}

var _ Txn = (*BadgerV2Txn)(nil)

type BadgerV2Txn struct {
	inner *badger.Txn
}

func (txn *BadgerV2Txn) Get(key Key) (Val, error) {
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

func (txn *BadgerV2Txn) Peek(key Key, f func(Val) error) error {
	switch item, err := txn.inner.Get(key); err {
	case nil:
		return item.Value(f)

	case badger.ErrKeyNotFound:
		return ErrKeyNotFound

	default:
		return fmt.Errorf("get value from badger: %w", err)
	}

}

func (txn *BadgerV2Txn) Put(key Key, val Val) error {
	return txn.inner.Set(key, val)
}

func (txn *BadgerV2Txn) Del(key Key) error {
	return txn.inner.Delete(key)
}

func (txn *BadgerV2Txn) Scan(prefix Prefix) (Iter, error) {
	it := txn.inner.NewIterator(badger.DefaultIteratorOptions)

	return &BadgerV2Iter{
		txn:    txn.inner,
		iter:   it,
		seeked: false,
		valid:  false,
		prefix: prefix,
	}, nil
}

var _ Iter = (*BadgerV2Iter)(nil)

type BadgerV2Iter struct {
	txn  *badger.Txn
	iter *badger.Iterator
	item *badger.Item

	seeked bool
	valid  bool
	prefix []byte
}

func (bi *BadgerV2Iter) Next() bool {
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

func (bi *BadgerV2Iter) Key() Key {
	if !bi.valid {
		return nil
	}

	return bi.item.Key()
}

func (bi *BadgerV2Iter) View(ctx context.Context, f func(Val) error) error {
	if !bi.valid {
		return ErrIterItemNotValid
	}

	return bi.item.Value(f)
}

func (bi *BadgerV2Iter) Close() {
	bi.iter.Close()
}

var _ DB = (*BadgerV2DB)(nil)

type BadgerV2DB struct {
	basePath string
	dbs      map[string]*badger.DB
	mu       sync.Mutex
}

func (db *BadgerV2DB) Run(context.Context) error {
	return nil
}

func (db *BadgerV2DB) Close(context.Context) error {
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

func (db *BadgerV2DB) OpenCollection(_ context.Context, name string) (KVStore, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if innerDB, ok := db.dbs[name]; ok {
		return &BadgerV2KVStore{db: innerDB}, nil
	}
	path := filepath.Join(db.basePath, name)
	opts := badger.DefaultOptions(path).WithLogger(&bV2logger{bV2log.With("path", path)})
	innerDB, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open sub badger %s, %w", name, err)
	}
	db.dbs[name] = innerDB
	return &BadgerV2KVStore{db: innerDB}, nil
}

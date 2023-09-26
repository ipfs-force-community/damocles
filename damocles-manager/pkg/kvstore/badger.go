package kvstore

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v2"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var _ KVStore = (*BadgerKVStore)(nil)

var blog = logging.New("kv").With("driver", "badger")

var (
	defaultGCSleep    = 20 * time.Second
	defaultGCInterval = 10 * time.Minute
)

type blogger struct {
	*logging.ZapLogger
}

func (bl *blogger) Warningf(format string, args ...interface{}) {
	bl.ZapLogger.Warnf(format, args...)
}

func OpenBadger(basePath string) DB {
	return &badgerDB{
		basePath: basePath,
		dbs:      make(map[string]*badger.DB),
		mu:       sync.Mutex{},
	}
}

type BadgerKVStore struct {
	db   *badger.DB
	name string
}

func (b *BadgerKVStore) View(_ context.Context, f func(Txn) error) error {
	err := b.db.View(func(txn *badger.Txn) error {
		return f(&BadgerTxn{inner: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (b *BadgerKVStore) Update(_ context.Context, f func(Txn) error) error {
	err := b.db.Update(func(txn *badger.Txn) error {
		return f(&BadgerTxn{inner: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (b *BadgerKVStore) NeedRetryTransactions() bool {
	return true
}

func (b *BadgerKVStore) Get(ctx context.Context, key Key) (val Val, err error) {
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

func (b *BadgerKVStore) Peek(ctx context.Context, key Key, f func(Val) error) error {
	for {
		err := b.View(ctx, func(txn Txn) error {
			return txn.Peek(key, f)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerKVStore) Put(ctx context.Context, key Key, val Val) error {
	for {
		err := b.Update(ctx, func(txn Txn) error {
			return txn.Put(key, val)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerKVStore) Del(ctx context.Context, key Key) error {
	for {
		err := b.Update(ctx, func(txn Txn) error {
			return txn.Del(key)
		})
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (b *BadgerKVStore) Scan(_ context.Context, prefix Prefix) (it Iter, err error) {
	txn := b.db.NewTransaction(false)
	iter := txn.NewIterator(badger.DefaultIteratorOptions)

	return &BadgerIterWithoutTrans{
		BadgerIter: &BadgerIter{
			txn:    txn,
			iter:   iter,
			seeked: false,
			valid:  false,
			prefix: prefix,
		}}, nil
}

func (b *BadgerKVStore) gc(ctx context.Context) {
	// use random time to avoid starting multiple gc tasks at the same time
	gcTimer := time.NewTimer(time.Duration(rand.Int63n(int64(defaultGCInterval))))
	for {
		select {
		case <-ctx.Done():
			return
		case <-gcTimer.C:
			start := time.Now()
			err := b.db.RunValueLogGC(0.7)
			switch err {
			case badger.ErrNoRewrite, badger.ErrRejected:
				// No rewrite means we've fully garbage collected.
				// Rejected means someone else is running a GC
				// or we're closing.
				gcTimer.Reset(defaultGCInterval)
			case nil:
				log.Infow("successful value log GC", "name", b.name, "elapsed", time.Since(start).Truncate(time.Microsecond).String())
				gcTimer.Reset(defaultGCSleep)
			default:
				log.Errorw("error duraing a GC cycle", "name", b.name, "err", err)
				gcTimer.Reset(defaultGCInterval)
			}

		}

	}
}

type BadgerIterWithoutTrans struct {
	*BadgerIter
}

func (bi *BadgerIterWithoutTrans) Close() {
	bi.BadgerIter.Close()
	bi.txn.Discard()
}

var _ Txn = (*BadgerTxn)(nil)

type BadgerTxn struct {
	inner *badger.Txn
}

func (txn *BadgerTxn) Get(key Key) (Val, error) {
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

func (txn *BadgerTxn) Peek(key Key, f func(Val) error) error {
	switch item, err := txn.inner.Get(key); err {
	case nil:
		return item.Value(f)

	case badger.ErrKeyNotFound:
		return ErrKeyNotFound

	default:
		return fmt.Errorf("get value from badger: %w", err)
	}

}

func (txn *BadgerTxn) Put(key Key, val Val) error {
	return txn.inner.Set(key, val)
}

func (txn *BadgerTxn) Del(key Key) error {
	return txn.inner.Delete(key)
}

func (txn *BadgerTxn) Scan(prefix Prefix) (Iter, error) {
	it := txn.inner.NewIterator(badger.DefaultIteratorOptions)

	return &BadgerIter{
		txn:    txn.inner,
		iter:   it,
		seeked: false,
		valid:  false,
		prefix: prefix,
	}, nil
}

var _ Iter = (*BadgerIter)(nil)

type BadgerIter struct {
	txn  *badger.Txn
	iter *badger.Iterator
	item *badger.Item

	seeked bool
	valid  bool
	prefix []byte
}

func (bi *BadgerIter) Next() bool {
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

func (bi *BadgerIter) Key() Key {
	if !bi.valid {
		return nil
	}

	return bi.item.Key()
}

func (bi *BadgerIter) View(_ context.Context, f func(Val) error) error {
	if !bi.valid {
		return ErrIterItemNotValid
	}

	return bi.item.Value(f)
}

func (bi *BadgerIter) Close() {
	bi.iter.Close()
}

var _ DB = (*badgerDB)(nil)

type badgerDB struct {
	basePath string
	dbs      map[string]*badger.DB
	mu       sync.Mutex
}

func (db *badgerDB) Run(context.Context) error {
	return nil
}

func (db *badgerDB) Close(context.Context) error {
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

func (db *badgerDB) OpenCollection(ctx context.Context, name string) (KVStore, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if innerDB, ok := db.dbs[name]; ok {
		return &BadgerKVStore{db: innerDB}, nil
	}
	path := filepath.Join(db.basePath, name)
	opts := badger.DefaultOptions(path).WithLogger(&blogger{blog.With("path", path)})
	innerDB, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open sub badger %s, %w", name, err)
	}
	db.dbs[name] = innerDB

	kv := &BadgerKVStore{db: innerDB, name: name}
	go kv.gc(ctx)
	return kv, nil
}

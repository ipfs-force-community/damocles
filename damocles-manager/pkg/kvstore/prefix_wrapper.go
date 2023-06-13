package kvstore

import (
	"context"
	"fmt"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("kv").With("driver", "prefix-wrapper")

var _ KVStore = (*WrappedKVStore)(nil)

func NewWrappedKVStore(prefix []byte, inner KVStore) (*WrappedKVStore, error) {
	prefixLen := len(prefix)
	if prefixLen == 0 {
		return nil, fmt.Errorf("empty prefix is not allowed")
	}

	if prefix[prefixLen-1] != '/' {
		prefixLen++
		p := make([]byte, prefixLen)

		// copied must be the size of prefix, thus it is also the last index of the p
		copied := copy(p, prefix)
		p[copied] = '/'
		prefix = p
	}

	log.Debugw("kv wrapped", "prefix", string(prefix), "prefix-len", prefixLen)

	return &WrappedKVStore{
		makeKey: makeKey{
			prefix:    prefix,
			prefixLen: prefixLen,
		},
		inner: inner,
	}, nil
}

type makeKey struct {
	prefix    []byte
	prefixLen int
}

func (p makeKey) makeKey(raw Key) Key {
	key := make(Key, p.prefixLen+len(raw))
	copy(key[:p.prefixLen], p.prefix)
	copy(key[p.prefixLen:], raw)
	return key
}

type WrappedKVStore struct {
	makeKey makeKey
	inner   KVStore
}

func (w *WrappedKVStore) View(ctx context.Context, f func(Txn) error) error {
	return w.inner.View(ctx, func(inner Txn) error {
		txn := &WrappedTxn{
			makeKey: w.makeKey,
			inner:   inner,
		}
		return f(txn)
	})
}

func (w *WrappedKVStore) Update(ctx context.Context, f func(Txn) error) error {
	return w.inner.Update(ctx, func(inner Txn) error {
		txn := &WrappedTxn{
			makeKey: w.makeKey,
			inner:   inner,
		}
		return f(txn)
	})
}

func (w *WrappedKVStore) NeedRetryTransactions() bool {
	return w.inner.NeedRetryTransactions()
}

func (w *WrappedKVStore) Get(ctx context.Context, key Key) (Val, error) {
	return w.inner.Get(ctx, w.makeKey.makeKey(key))
}

func (w *WrappedKVStore) Peek(ctx context.Context, key Key, f func(Val) error) error {
	return w.inner.Peek(ctx, w.makeKey.makeKey(key), f)
}

func (w *WrappedKVStore) Put(ctx context.Context, key Key, val Val) error {
	return w.inner.Put(ctx, w.makeKey.makeKey(key), val)
}

func (w *WrappedKVStore) Del(ctx context.Context, key Key) error {
	return w.inner.Del(ctx, w.makeKey.makeKey(key))
}

func (w *WrappedKVStore) Scan(ctx context.Context, prefix Prefix) (Iter, error) {
	iter, err := w.inner.Scan(ctx, w.makeKey.makeKey(prefix))
	if err != nil {
		return nil, err
	}

	return &WrappedIter{
		prefixLen: w.makeKey.prefixLen,
		inner:     iter,
	}, nil
}

var _ Txn = (*WrappedTxn)(nil)

type WrappedTxn struct {
	makeKey makeKey
	inner   Txn
}

func (wt *WrappedTxn) Get(key Key) (Val, error) {
	return wt.inner.Get(wt.makeKey.makeKey(key))
}

func (wt *WrappedTxn) Peek(key Key, f func(Val) error) error {
	return wt.inner.Peek(wt.makeKey.makeKey(key), f)
}

func (wt *WrappedTxn) Put(key Key, val Val) error {
	return wt.inner.Put(wt.makeKey.makeKey(key), val)
}

func (wt *WrappedTxn) Del(key Key) error {
	return wt.inner.Del(wt.makeKey.makeKey(key))
}

func (wt *WrappedTxn) Scan(prefix Prefix) (Iter, error) {
	iter, err := wt.inner.Scan(wt.makeKey.makeKey(prefix))
	if err != nil {
		return nil, err
	}

	return &WrappedIter{
		prefixLen: wt.makeKey.prefixLen,
		inner:     iter,
	}, nil
}

var _ Iter = (*WrappedIter)(nil)

type WrappedIter struct {
	prefixLen int
	inner     Iter
}

func (wi *WrappedIter) Next() bool { return wi.inner.Next() }

func (wi *WrappedIter) View(ctx context.Context, f func(Val) error) error {
	return wi.inner.View(ctx, f)
}

func (wi *WrappedIter) Close() { wi.inner.Close() }

func (wi *WrappedIter) Key() Key {
	key := wi.inner.Key()
	if len(key) > wi.prefixLen {
		key = key[wi.prefixLen:]
	}

	return key
}

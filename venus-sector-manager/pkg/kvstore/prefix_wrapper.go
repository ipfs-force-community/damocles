package kvstore

import (
	"context"
	"fmt"
)

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
		prefix:    prefix,
		prefixLen: prefixLen,
		inner:     inner,
	}, nil
}

type WrappedKVStore struct {
	prefix    []byte
	prefixLen int
	inner     KVStore
}

func (w *WrappedKVStore) makeKey(raw Key) Key {
	key := make(Key, w.prefixLen+len(raw))
	copy(key[:w.prefixLen], w.prefix)
	copy(key[w.prefixLen:], raw)
	return key
}

func (w *WrappedKVStore) Get(ctx context.Context, key Key) (Val, error) {
	return w.inner.Get(ctx, w.makeKey(key))
}

func (w *WrappedKVStore) Has(ctx context.Context, key Key) (bool, error) {
	return w.inner.Has(ctx, w.makeKey(key))
}

func (w *WrappedKVStore) View(ctx context.Context, key Key, cb Callback) error {
	return w.inner.View(ctx, w.makeKey(key), cb)
}

func (w *WrappedKVStore) Put(ctx context.Context, key Key, val Val) error {
	return w.inner.Put(ctx, w.makeKey(key), val)
}

func (w *WrappedKVStore) Del(ctx context.Context, key Key) error {
	return w.inner.Del(ctx, w.makeKey(key))
}

func (w *WrappedKVStore) Scan(ctx context.Context, prefix Prefix) (Iter, error) {
	iter, err := w.inner.Scan(ctx, w.makeKey(prefix))
	if err != nil {
		return nil, err
	}

	return &WrappedIter{
		prefixLen: w.prefixLen,
		inner:     iter,
	}, nil
}

func (w *WrappedKVStore) Run(ctx context.Context) error {
	return nil
}

func (w *WrappedKVStore) Close(ctx context.Context) error {
	return nil
}

type WrappedIter struct {
	prefixLen int
	inner     Iter
}

func (wi *WrappedIter) Next() bool                                  { return wi.inner.Next() }
func (wi *WrappedIter) View(ctx context.Context, cb Callback) error { return wi.inner.View(ctx, cb) }
func (wi *WrappedIter) Close()                                      { wi.inner.Close() }

func (wi *WrappedIter) Key() Key {
	key := wi.inner.Key()
	if len(key) > wi.prefixLen {
		key = key[wi.prefixLen:]
	}

	return key
}

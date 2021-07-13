package kvstore

import "context"

var _ KVStore = (*WrappedKVStore)(nil)

func NewWrappedKVStore(prefix []byte, inner KVStore) *WrappedKVStore {
	return &WrappedKVStore{
		prefix:    prefix,
		prefixLen: len(prefix),
		inner:     inner,
	}
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

func (w *WrappedKVStore) Run(ctx context.Context) error {
	return nil
}

func (w *WrappedKVStore) Close(ctx context.Context) error {
	return nil
}

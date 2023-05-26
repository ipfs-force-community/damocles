package main

import (
	"bytes"
	"context"
	"errors"
	"sync"

	vsmplugin "github.com/ipfs-force-community/venus-cluster/vsm-plugin"
	"github.com/ipfs-force-community/venus-cluster/vsm-plugin/kvstore"
	"github.com/tidwall/btree"
)

var (
	_ kvstore.KVStore = (*collection)(nil)
	_ kvstore.Iter    = (*iter)(nil)
	_ kvstore.DB      = (*memdb)(nil)
)

func OnInit(ctx context.Context, pluginsDir string, manifest *vsmplugin.Manifest) error { return nil }

func Open(meta map[string]string) (kvstore.DB, error) {
	return &memdb{
		collections: make(map[string]*collection),
		lock:        sync.Mutex{},
	}, nil
}

type memdb struct {
	collections map[string]*collection
	lock        sync.Mutex
}

func (*memdb) Close(context.Context) error {
	return nil
}

func (m *memdb) OpenCollection(_ context.Context, name string) (kvstore.KVStore, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	kv, ok := m.collections[name]
	if !ok {
		kv = &collection{
			inner: btree.NewMap[string, kvstore.Val](32),
			lock:  sync.RWMutex{},
		}
		m.collections[name] = kv
	}
	return kv, nil
}

func (*memdb) Run(context.Context) error {
	return nil
}

type collection struct {
	inner *btree.Map[string, kvstore.Val]
	lock  sync.RWMutex
}

func (c *collection) View(ctx context.Context, f func(kvstore.Txn) error) error {
	return errors.New("the memdb does not support transaction")
}

func (c *collection) Update(ctx context.Context, f func(kvstore.Txn) error) error {
	return errors.New("the memdb does not support transaction")
}

func (c *collection) NeedRetryTransactions() bool {
	return false
}

func (c *collection) Get(_ context.Context, k kvstore.Key) (kvstore.Val, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	v, ok := c.inner.Get(string(k))
	if !ok {
		return nil, kvstore.ErrKeyNotFound
	}
	return v, nil
}

func (c *collection) Peek(ctx context.Context, k kvstore.Key, f func(kvstore.Val) error) error {
	v, err := c.Get(ctx, k)
	if err != nil {
		return err
	}
	return f(v)
}

func (c *collection) Put(_ context.Context, k kvstore.Key, v kvstore.Val) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.inner.Set(string(k), v)
	return nil
}

func (c *collection) Del(_ context.Context, k kvstore.Key) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.inner.Delete(string(k))
	return nil
}

func (c *collection) Scan(_ context.Context, prefix kvstore.Prefix) (kvstore.Iter, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	innerIt := c.inner.Iter()
	return &iter{
		inner:  innerIt,
		prefix: prefix,
	}, nil
}

type iter struct {
	inner  btree.MapIter[string, kvstore.Val]
	prefix kvstore.Prefix
	seeked bool
}

func (it *iter) Next() bool {
	var next bool
	if !it.seeked {
		if !it.inner.Seek(string(it.prefix)) {
			next = it.inner.Last()
		} else {
			next = true
		}
		it.seeked = true
	} else {
		next = it.inner.Next()
	}
	if !next {
		return false
	}
	return len(it.prefix) == 0 || bytes.HasPrefix(it.Key(), it.prefix)
}

func (it *iter) Key() kvstore.Key {
	return kvstore.Key(it.inner.Key())
}

func (it *iter) View(_ context.Context, f func(kvstore.Val) error) error {
	return f(it.inner.Value())
}

func (it *iter) Close() {}

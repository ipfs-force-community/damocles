package kvstore

import (
	"context"
	"encoding/json"
	"errors"

	pluginkvstore "github.com/ipfs-force-community/damocles/manager-plugin/kvstore"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var Log = logging.New("kv")

var (
	ErrKeyNotFound         = pluginkvstore.ErrKeyNotFound
	ErrIterItemNotValid    = pluginkvstore.ErrIterItemNotValid
	ErrTransactionConflict = pluginkvstore.ErrTransactionConflict
)

type (
	Key    = pluginkvstore.Key
	Val    = pluginkvstore.Val
	Prefix = pluginkvstore.Prefix

	KVStore = pluginkvstore.KVStore
	DB      = pluginkvstore.DB
	Iter    = pluginkvstore.Iter
	Txn     = pluginkvstore.Txn
)

var LoadJSON = func(target any) func(Val) error {
	return func(data Val) error {
		return json.Unmarshal(data, target)
	}
}

var NilF = func(Val) error {
	return nil
}

func NewKVExt(kvStore KVStore) *KVExt {
	return &KVExt{
		KVStore: kvStore,
	}
}

type KVExt struct {
	KVStore
}

func (kv *KVExt) MustNoConflict(f func() error) error {
	if !kv.NeedRetryTransactions() {
		return f()
	}
	for {
		err := f()
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

func (kv *KVExt) UpdateMustNoConflict(ctx context.Context, f func(txn TxnExt) error) error {
	return kv.MustNoConflict(func() error {
		return kv.Update(ctx, func(t Txn) error {
			return f(TxnExt{Txn: t})
		})
	})
}

func (kv *KVExt) ViewMustNoConflict(ctx context.Context, f func(txn TxnExt) error) error {
	return kv.MustNoConflict(func() error {
		return kv.View(ctx, func(t Txn) error {
			return f(TxnExt{Txn: t})
		})
	})
}

type TxnExt struct {
	Txn
}

func (et TxnExt) PeekAny(f func(Val) error, keys ...Key) (Key, error) {
	for _, k := range keys {
		err := et.Peek(k, f)
		if errors.Is(err, ErrKeyNotFound) {
			continue
		}
		return k, err
	}
	return []byte{}, ErrKeyNotFound
}

func (et TxnExt) PutJSON(k Key, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return et.Put(k, b)
}

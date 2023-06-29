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

var LoadJson = func(target any) func(Val) error {
	return func(data Val) error {
		return json.Unmarshal(data, target)
	}
}

func NewExtendKV(kvStore KVStore) *ExtendKV {
	return &ExtendKV{
		KVStore: kvStore,
	}
}

type ExtendKV struct {
	KVStore
}

func (kv *ExtendKV) MustNoConflict(f func() error) error {
	if kv.NeedRetryTransactions() {
		for {
			err := f()
			if !errors.Is(err, ErrTransactionConflict) {
				return err
			}
		}
	} else {
		return f()
	}
}

func (kv *ExtendKV) UpdateMustNoConflict(ctx context.Context, f func(txn ExtendTxn) error) error {
	return kv.MustNoConflict(func() error {
		return kv.Update(ctx, func(t Txn) error {
			return f(ExtendTxn{Txn: t})
		})
	})
}

func (kv *ExtendKV) ViewMustNoConflict(ctx context.Context, f func(txn ExtendTxn) error) error {
	return kv.MustNoConflict(func() error {
		return kv.View(ctx, func(t Txn) error {
			return f(ExtendTxn{Txn: t})
		})
	})
}

type ExtendTxn struct {
	Txn
}

func (et ExtendTxn) PeekAny(f func(Val) error, keys ...Key) error {
	for _, k := range keys {
		err := et.Peek(k, f)
		if errors.Is(err, ErrKeyNotFound) {
			continue
		}
		return err
	}
	return ErrKeyNotFound
}

func (et ExtendTxn) PutJson(k Key, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return et.Put(k, b)
}

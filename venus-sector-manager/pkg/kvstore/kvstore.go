package kvstore

import (
	"errors"

	pluginkvstore "github.com/ipfs-force-community/venus-cluster/vsm-plugin/kvstore"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
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

func NewKVStoreExtend(kvStore KVStore) *KVStoreExtend {
	return &KVStoreExtend{
		KVStore: kvStore,
	}
}

type KVStoreExtend struct {
	KVStore
}

func (kv *KVStoreExtend) MustNoConflict(f func() error) error {
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

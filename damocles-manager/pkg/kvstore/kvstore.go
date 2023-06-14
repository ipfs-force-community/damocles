package kvstore

import (
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

func NewExtend(kvStore KVStore) *Extend {
	return &Extend{
		KVStore: kvStore,
	}
}

type Extend struct {
	KVStore
}

func (kv *Extend) MustNoConflict(f func() error) error {
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

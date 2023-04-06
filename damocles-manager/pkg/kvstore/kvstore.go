package kvstore

import (
	pluginkvstore "github.com/ipfs-force-community/damocles/damocles-manager-plugin/kvstore"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var Log = logging.New("kv")

var (
	ErrKeyNotFound      = pluginkvstore.ErrKeyNotFound
	ErrIterItemNotValid = pluginkvstore.ErrIterItemNotValid
)

type (
	Key      = pluginkvstore.Key
	Val      = pluginkvstore.Val
	Prefix   = pluginkvstore.Prefix
	Callback = pluginkvstore.Callback

	KVStore = pluginkvstore.KVStore
	DB      = pluginkvstore.DB
	Iter    = pluginkvstore.Iter
)

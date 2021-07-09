package types

import (
	"github.com/ipfs/go-datastore"
)

type Key = datastore.Key

var NewKey = datastore.NewKey
var RawKey = datastore.RawKey
var ErrKeyNotFound = datastore.ErrNotFound

type KVStore interface {
	datastore.Datastore
	View(key datastore.Key, callback func([]byte) error) error
}

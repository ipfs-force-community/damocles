package kvstore

import (
	"context"
	"fmt"
)

var (
	ErrKeyNotFound      = fmt.Errorf("key not found")
	ErrIterItemNotValid = fmt.Errorf("iter item not valid")
)

type (
	Key      = []byte
	Val      = []byte
	Prefix   = []byte
	Callback = func(Val) error
)

// Iter is not guaranteed to be thread-safe
type Iter interface {
	Next() bool
	Key() Key
	View(context.Context, Callback) error
	Close()
}

type KVStore interface {
	Get(context.Context, Key) (Val, error)
	Has(context.Context, Key) (bool, error)
	View(context.Context, Key, Callback) error
	Put(context.Context, Key, Val) error
	Del(context.Context, Key) error

	// in most implementations, scan will hold a read lock
	Scan(context.Context, Prefix) (Iter, error)
}

type DB interface {
	Run(context.Context) error
	Close(context.Context) error

	// OpenCollection opens a collection with the given name and returns a KVStore,
	// creating it if needed.
	OpenCollection(ctx context.Context, name string) (KVStore, error)
}

package kvstore

import (
	"context"
	"fmt"
)

var ErrKeyNotFound = fmt.Errorf("key not found")

type (
	Key      []byte
	Val      = []byte
	Callback = func(Val) error
)

func (k Key) String() string {
	return string(k)
}

type KVStore interface {
	Get(context.Context, Key) (Val, error)
	Has(context.Context, Key) (bool, error)
	View(context.Context, Key, Callback) error
	Put(context.Context, Key, Val) error
	Del(context.Context, Key) error
	Run(context.Context) error
	Close(context.Context) error
}

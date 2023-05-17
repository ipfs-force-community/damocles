package kvstore

import (
	"context"
	"fmt"
)

var (
	ErrKeyNotFound         = fmt.Errorf("key not found")
	ErrIterItemNotValid    = fmt.Errorf("iter item not valid")
	ErrTransactionConflict = fmt.Errorf("transaction conflict")
)

type (
	Key    = []byte
	Val    = []byte
	Prefix = []byte
)

// Iter is not guaranteed to be thread-safe
type Iter interface {
	Next() bool
	Key() Key
	View(context.Context, func(Val) error) error
	Close()
}

type Txn interface {
	Get(Key) (Val, error)
	Peek(Key, func(Val) error) error
	Put(Key, Val) error
	Del(Key) error
	Scan(Prefix) (Iter, error)
}

type KVStore interface {
	View(context.Context, func(Txn) error) error
	Update(context.Context, func(Txn) error) error

	// NeedRetryTransactions returns whether manual retry is required when a database conflict occurs.
	// In general, when using optimistic transaction model databases, transactions are prone to conflict in heavy contention scenarios.
	// If a conflict is detected, an ErrTransactionConflict error will be returned immediately, resulting in a failed submission.
	// At this time, depending on the state of your application, you have the option to retry the operation if you receive this error.
	// However, some database client drivers already include retry logic in their implementation. Therefore, manual retry in the application is not necessary.
	// For example, use the `WithTransaction` method of mongo-go-driver
	NeedRetryTransactions() bool

	Get(context.Context, Key) (Val, error)
	Peek(context.Context, Key, func(Val) error) error
	Put(context.Context, Key, Val) error
	Del(context.Context, Key) error
	Scan(context.Context, Prefix) (Iter, error)
}

type DB interface {
	Run(context.Context) error
	Close(context.Context) error

	// OpenCollection opens a collection with the given name and returns a KVStore,
	// creating it if needed.
	OpenCollection(ctx context.Context, name string) (KVStore, error)
}

package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/0x5459/badger/v4"
)

var (
	_ Store = (*badgerStore)(nil)
	_ Txn   = (*badgerTxn)(nil)
)

var (
	ErrKeyNotFound         = fmt.Errorf("key not found")
	ErrTransactionConflict = fmt.Errorf("transaction conflict")
)

func Update(s Store, f func(Txn) error) error {
	for {
		err := s.Update(f)
		if !errors.Is(err, ErrTransactionConflict) {
			return err
		}
	}
}

type Store interface {
	// Update starts Read-write transactions
	Update(func(Txn) error) error
	Close() error
}

type Txn interface {
	Set(key string, value string, ttl time.Duration) error
	Get(key string) (value string, err error)
	Del(key string) error
}

func NewBadgerStore(path string) (Store, error) {
	db, err := badger.Open(badger.DefaultOptions(path))
	if err != nil {
		return nil, err
	}
	return &badgerStore{
		db: db,
	}, nil
}

type badgerStore struct {
	db *badger.DB
}

// Update starts Read-write transactions
func (bs *badgerStore) Update(f func(s Txn) error) error {
	err := bs.db.Update(func(txn *badger.Txn) error {
		return f(&badgerTxn{txn: txn})
	})
	if errors.Is(err, badger.ErrConflict) {
		return ErrTransactionConflict
	}
	return err
}

func (bs *badgerStore) Close() error {
	return bs.db.Close()
}

type badgerTxn struct {
	txn *badger.Txn
}

func (bt *badgerTxn) Set(key string, value string, ttl time.Duration) error {
	e := badger.NewEntry([]byte(key), []byte(value)).WithTTL(ttl)
	return bt.txn.SetEntry(e)
}

func (bt *badgerTxn) Get(key string) (value string, err error) {

	var val []byte
	switch item, err := bt.txn.Get([]byte(key)); err {
	case nil:
		val, err = item.ValueCopy(nil)
		if err != nil {
			return "", err
		}
		return string(val), nil

	case badger.ErrKeyNotFound:
		return "", ErrKeyNotFound

	default:
		return "", fmt.Errorf("get value from badger: %w", err)
	}
}

func (bt *badgerTxn) Del(key string) error {
	return bt.txn.Delete([]byte(key))
}

func HasKey(t Txn, key string) (bool, error) {
	switch _, err := t.Get(key); err {
	case nil:
		return true, nil
	case ErrKeyNotFound:
		return false, nil
	default:
		return false, err
	}
}

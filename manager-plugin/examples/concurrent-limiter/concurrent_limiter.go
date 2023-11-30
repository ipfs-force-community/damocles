package main

import (
	"fmt"
	"strings"
	"time"
)

func NewConcurrentLimiter(
	concurrent map[string]uint16,
	slotTimeout time.Duration,
	store Store,
) *ConcurrentLimiter {
	return &ConcurrentLimiter{
		concurrent:  concurrent,
		slotTimeout: slotTimeout,
		store:       store,
	}
}

type ConcurrentLimiter struct {
	// name -> concurrent
	concurrent  map[string]uint16
	slotTimeout time.Duration
	store       Store
}

func (l *ConcurrentLimiter) Acquire(name string, id string) (string, error) {

	slots := l.slots(name)
	if len(slots) == 0 {
		return "", fmt.Errorf("invalid name: %s", name)
	}

	targetSlot := ""
	err := Update(l.store, func(t Txn) error {
		for _, slot := range slots {
			has, err := HasKey(t, slot)
			if err != nil {
				return err
			}
			if !has {
				err := t.Set(slot, id, l.slotTimeout)
				if err != nil {
					return err
				}
				targetSlot = slot
				break
			}
		}
		return nil
	})
	return targetSlot, err
}

func (l *ConcurrentLimiter) Release(slot, id string) error {
	if !l.checkSlot(slot) {
		return fmt.Errorf("invalid slot %s", slot)
	}

	return Update(l.store, func(t Txn) error {
		v, err := t.Get(slot)
		if err == ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if v == id {
			return t.Del(slot)
		}
		return nil
	})
}

func (l *ConcurrentLimiter) Extend(slot, id string) (ok bool, err error) {
	if !l.checkSlot(slot) {
		return false, nil
	}
	err = Update(l.store, func(t Txn) error {
		val, err := t.Get(slot)
		if err != nil && err != ErrKeyNotFound {
			ok = false
			return err
		}
		if err == ErrKeyNotFound || val == id {
			err = t.Set(slot, id, l.slotTimeout)
			ok = err == nil
			return err
		}
		ok = false
		return nil
	})
	return
}

func (l *ConcurrentLimiter) slots(name string) []string {
	concurrent, ok := l.concurrent[name]
	if !ok {
		return []string{}
	}

	slots := make([]string, concurrent)
	for i := 0; i < int(concurrent); i++ {
		slots[i] = fmt.Sprintf("%d-%s", i, name)
	}
	return slots
}

func (l *ConcurrentLimiter) checkSlot(slot string) bool {
	s := strings.SplitN(slot, "-", 2)
	if len(s) < 2 {
		return false
	}
	_, ok := l.concurrent[s[1]]
	return ok
}

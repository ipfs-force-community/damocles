package main_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	limiter "gitlab.forceup.in/ForceMining/force-ext-processors/vsm-plugins/concurrent-limiter"
)

func TestTxn_Get(t *testing.T) {
	s := testStore(t)

	limiter.Update(s, func(txn limiter.Txn) error {
		require.NoError(t, txn.Set("hello", "world", 1000*time.Hour))
		v, err := txn.Get("hello")
		require.NoError(t, err)
		require.Equal(t, "world", v)

		require.NoError(t, txn.Set("hello", "abc", 1000*time.Hour))
		v, err = txn.Get("hello")
		require.NoError(t, err)
		require.Equal(t, "abc", v)

		require.NoError(t, txn.Set("你好", "世界", 1000*time.Hour))
		v, err = txn.Get("你好")
		require.NoError(t, err)
		require.Equal(t, "世界", v)

		_, err = txn.Get("💗")
		require.Equal(t, limiter.ErrKeyNotFound, err)
		return nil
	})
}

func TestTxn_Del(t *testing.T) {
	s := testStore(t)

	limiter.Update(s, func(txn limiter.Txn) error {
		require.NoError(t, txn.Del("a"))
		require.NoError(t, txn.Set("aa", "aa", 1000*time.Hour))
		has, err := limiter.HasKey(txn, "aa")
		require.NoError(t, err)
		require.True(t, has)

		require.NoError(t, txn.Del("aa"))
		has, err = limiter.HasKey(txn, "aa")
		require.NoError(t, err)
		require.False(t, has)
		return nil
	})
}

func TestTxnTTL(t *testing.T) {
	s := testStore(t)

	err := limiter.Update(s, func(txn limiter.Txn) error {
		return txn.Set("hello", "value", time.Second)
	})
	require.NoError(t, err)

	limiter.Update(s, func(txn limiter.Txn) error {
		has, err := limiter.HasKey(txn, "hello")
		require.NoError(t, err)
		require.True(t, has)
		return nil
	})

	time.Sleep(time.Second)

	limiter.Update(s, func(txn limiter.Txn) error {
		has, err := limiter.HasKey(txn, "hello")
		require.NoError(t, err)
		require.False(t, has)
		return nil
	})

}

func testStore(t *testing.T) limiter.Store {
	tmpdir := t.TempDir()
	s, err := limiter.NewBadgerStore(tmpdir)
	require.NoError(t, err)
	t.Cleanup(func() {
		s.Close()
	})
	return s
}

package kvstore_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil"
	"github.com/ipfs-force-community/venus-cluster/vsm-plugin/kvstore"
	"github.com/stretchr/testify/require"
)

func TestBadger_PutAndGet(t *testing.T) {
	ctx := context.TODO()
	kv := testutil.BadgerKVStore(t, "test")

	require.NoError(t, kv.Put(ctx, b("hello"), b("world")))
	v, err := kv.Get(ctx, b("hello"))
	require.NoError(t, err)
	require.Equal(t, b("world"), v)

	require.NoError(t, kv.Put(ctx, b("hello"), b("abc")))
	v, err = kv.Get(ctx, b("hello"))
	require.NoError(t, err)
	require.Equal(t, b("abc"), v)

	require.NoError(t, kv.Put(ctx, b("你好"), b("世界")))
	v, err = kv.Get(ctx, b("你好"))
	require.NoError(t, err)
	require.Equal(t, b("世界"), v)

	_, err = kv.Get(ctx, b("💗"))
	require.Equal(t, kvstore.ErrKeyNotFound, err)
}

func TestBadger_Del(t *testing.T) {
	ctx := context.TODO()
	kv := testutil.BadgerKVStore(t, "test")

	require.NoError(t, kv.Del(ctx, b("a")))
	require.NoError(t, kv.Put(ctx, b("aa"), b("aa")))
	_, err := kv.Get(ctx, b("aa"))
	require.NoError(t, err)

	require.NoError(t, kv.Del(ctx, b("aa")))
	_, err = kv.Get(ctx, b("aa"))
	require.ErrorIs(t, err, kvstore.ErrKeyNotFound)
}

func TestBadger_Scan(t *testing.T) {
	ctx := context.TODO()
	kv := testutil.BadgerKVStore(t, "test")

	for _, k := range []string{"a1", "a2", "a3", "b1", "b2", "b3"} {
		require.NoError(t, kv.Put(ctx, b(k), b(k+"_v")))
	}

	cases := []struct {
		prefix   kvstore.Prefix
		expected []entry
	}{
		{
			prefix: b("a"),
			expected: []entry{
				{k: b("a1"), v: b("a1_v")},
				{k: b("a2"), v: b("a2_v")},
				{k: b("a3"), v: b("a3_v")},
			},
		},
		{
			prefix: b("b"),
			expected: []entry{
				{k: b("b1"), v: b("b1_v")},
				{k: b("b2"), v: b("b2_v")},
				{k: b("b3"), v: b("b3_v")},
			},
		},
		{
			prefix: nil,
			expected: []entry{
				{k: b("a1"), v: b("a1_v")},
				{k: b("a2"), v: b("a2_v")},
				{k: b("a3"), v: b("a3_v")},
				{k: b("b1"), v: b("b1_v")},
				{k: b("b2"), v: b("b2_v")},
				{k: b("b3"), v: b("b3_v")},
			},
		},
	}

	for _, c := range cases {
		func() {
			it, err := kv.Scan(ctx, c.prefix)
			require.NoError(t, err)
			defer it.Close()

			actual, err := all(ctx, it)
			require.NoError(t, err)
			require.ElementsMatch(t, c.expected, actual, "test for prefix: %s", string(c.prefix))
		}()
	}
}

func TestBadgerTxn_Commit(t *testing.T) {
	ctx := context.TODO()
	kv := testutil.BadgerKVStore(t, "test")

	err := kv.Update(ctx, func(txn kvstore.Txn) error {
		require.NoError(t, txn.Put(testKey1, testValue1))
		_, err := txn.Get(testKey1)
		require.NoError(t, err)
		return nil
	})
	require.NoError(t, err)
	_, err = kv.Get(ctx, testKey1)
	require.NoError(t, err)
}

func TestBadgerTxn_Rollback(t *testing.T) {
	ctx := context.TODO()
	kv := testutil.BadgerKVStore(t, "test")

	err := kv.Update(ctx, func(txn kvstore.Txn) error {
		require.NoError(t, txn.Put(testKey1, testValue1))
		_, err := txn.Get(testKey1)
		require.NoError(t, err)
		return errors.New("some error")
	})
	require.NotNil(t, err)
	_, err = kv.Get(ctx, testKey1)
	require.ErrorIs(t, err, kvstore.ErrKeyNotFound)
}

func b(x string) []byte {
	return []byte(x)
}

type entry struct {
	k kvstore.Key
	v kvstore.Val
}

func all(ctx context.Context, it kvstore.Iter) ([]entry, error) {
	vals := []entry{}

	for it.Next() {
		var val kvstore.Val
		it.View(ctx, func(v kvstore.Val) error {
			val = v
			return nil
		})
		vals = append(vals, entry{
			k: it.Key(),
			v: val,
		})
	}
	return vals, nil
}

package kvstore_test

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/strikesecurity/strikememongo"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var (
	mongoServer   *strikememongo.Server
	testKey1      = []byte("testKey1")
	testKey2      = []byte("testKey2")
	testKey3      = []byte("testKey3")
	testPrefixKey = []byte("testKey")

	testValue1 = []byte("testValue1")
	testValue2 = []byte("testValue2")
	testValue3 = []byte("testValue3")
)

func TestMain(m *testing.M) {
	var err error
	mongoServer, err = strikememongo.Start("4.0.5")
	if err != nil {
		log.Fatal(err)
	}
	defer mongoServer.Stop()

	os.Exit(m.Run())
}

func DeleteAll(ctx context.Context, kv kvstore.KVStore) error {
	iter, err := kv.Scan(ctx, nil)
	if err != nil {
		return fmt.Errorf("Scan all record: %w", err)

	}
	for iter.Next() {
		k := iter.Key()
		if err := kv.Del(ctx, k); err != nil {
			return fmt.Errorf("Delete key: %s;  %w", string(k), err)
		}
	}
	return nil
}

func TestMongoStore_PutGet(t *testing.T) {
	ctx := context.TODO()
	kv := testMongoKV(ctx, t, "test")

	require.NoError(t, DeleteAll(ctx, kv))

	err := kv.Put(ctx, testKey1, testValue1)
	require.NoError(t, err)

	val, err := kv.Get(ctx, testKey1)
	require.NoError(t, err)
	require.Equal(t, testValue1, val)

	_, err = kv.Get(ctx, testKey2)
	require.Equal(t, kvstore.ErrKeyNotFound, err)

	err = kv.Put(ctx, testKey1, testValue2)
	require.NoError(t, err)

	val, err = kv.Get(ctx, testKey1)
	require.NoError(t, err)
	require.Equal(t, testValue2, val)
}

func TestMongoStore_Has(t *testing.T) {
	ctx := context.TODO()
	kv := testMongoKV(ctx, t, "test")

	require.NoError(t, DeleteAll(ctx, kv))

	err := kv.Put(ctx, testKey1, testValue1)
	require.NoError(t, err)

	exist, err := kv.Has(ctx, testKey1)
	require.NoError(t, err)
	require.Equal(t, true, exist)

	exist, err = kv.Has(ctx, testKey2)
	require.NoError(t, err)
	require.Equal(t, false, exist)
}

// this case will also test the usage of iter
func TestMongoStore_Scan(t *testing.T) {
	ctx := context.TODO()
	kv := testMongoKV(ctx, t, "test")

	require.NoError(t, DeleteAll(ctx, kv))

	err := kv.Put(ctx, testKey1, testValue1)
	require.NoError(t, err)
	err = kv.Put(ctx, testKey2, testValue2)
	require.NoError(t, err)
	err = kv.Put(ctx, testKey3, testValue3)
	require.NoError(t, err)

	iter, err := kv.Scan(ctx, testPrefixKey)
	require.NoError(t, err)

	cnt := 0
	for iter.Next() {
		cnt++
		v := kvstore.Val{}
		err = iter.View(ctx, func(val kvstore.Val) error {
			v = val
			return nil
		})
		require.NoError(t, err)
		switch {
		case bytes.Equal(iter.Key(), testKey1):
			require.Equal(t, testValue1, v)
		case bytes.Equal(iter.Key(), testKey2):
			require.Equal(t, testValue2, v)
		case bytes.Equal(iter.Key(), testKey3):
			require.Equal(t, testValue3, v)
		default:
			require.Error(t, fmt.Errorf("failed to match iter.Key"))
		}
	}
	require.Equal(t, 3, cnt)
}

func TestMongoStore_ScanNil(t *testing.T) {
	ctx := context.TODO()
	kv := testMongoKV(ctx, t, "test")

	require.NoError(t, DeleteAll(ctx, kv))

	err := kv.Put(ctx, testKey1, testValue1)
	require.NoError(t, err)
	err = kv.Put(ctx, testKey2, testValue2)
	require.NoError(t, err)
	err = kv.Put(ctx, testKey3, testValue3)
	require.NoError(t, err)

	err = kv.Put(ctx, []byte("tmp"), testValue3)
	require.NoError(t, err)
	// should scan all key
	iter, err := kv.Scan(ctx, nil)
	require.NoError(t, err)

	cnt := 0
	for iter.Next() {
		cnt++
	}
	require.Equal(t, 4, cnt)
}

func TestMongoStore_Del(t *testing.T) {
	ctx := context.TODO()
	kv := testMongoKV(ctx, t, "test")

	require.NoError(t, DeleteAll(ctx, kv))

	err := kv.Put(ctx, testKey1, testValue1)
	require.NoError(t, err)
	err = kv.Put(ctx, testKey2, testValue2)
	require.NoError(t, err)
	err = kv.Del(ctx, testKey2)
	require.NoError(t, err)

	iter, err := kv.Scan(ctx, nil)
	require.NoError(t, err)

	cnt := 0
	for iter.Next() {
		cnt++
	}
	require.Equal(t, 1, cnt)

	_, err = kv.Get(ctx, testKey1)
	require.NoError(t, err)

	_, err = kv.Get(ctx, testKey2)
	require.Equal(t, kvstore.ErrKeyNotFound, err)
}

func testMongoKV(ctx context.Context, t *testing.T, collection string) kvstore.KVStore {
	db, err := kvstore.OpenMongo(context.TODO(), mongoServer.URI(), "vcs")
	require.NoError(t, err)
	require.NoError(t, db.Run(ctx))
	kv, err := db.OpenCollection(ctx, collection)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Close(ctx)
	})
	return kv
}

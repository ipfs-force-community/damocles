package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

func TestKVStore(t *testing.T, collection string) kvstore.KVStore {
	ctx := context.Background()
	tmpdir := t.TempDir()
	db := kvstore.OpenBadger(tmpdir)
	require.NoError(t, db.Run(ctx))
	kv, err := db.OpenCollection(ctx, collection)
	require.NoErrorf(t, err, "open badger at %s/%s", tmpdir, collection)
	t.Cleanup(func() {
		db.Close(ctx)
	})
	return kv
}

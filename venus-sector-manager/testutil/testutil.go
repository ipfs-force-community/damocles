package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

func TestKVStore(t *testing.T, collection string) (kvstore.KVStore, func()) {
	tmpdir := t.TempDir()
	db := kvstore.OpenBadger(tmpdir)
	kv, err := db.OpenCollection(collection)
	require.NoErrorf(t, err, "open badger at %s/%s", tmpdir, collection)
	return kv, func() {
		db.Close(context.Background())
	}
}

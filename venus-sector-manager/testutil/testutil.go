package testutil

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

func TestKVStore(t *testing.T) (kvstore.KVStore, func()) {
	tmpdir := t.TempDir()
	store, err := kvstore.OpenBadger(kvstore.DefaultBadgerOption(tmpdir))
	require.NoErrorf(t, err, "open badger at %s", tmpdir)
	return store, func() {
		store.Close(context.Background())
	}
}

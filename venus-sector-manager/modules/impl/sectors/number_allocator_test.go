package sectors

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil"
)

func TestAllocatorNext(t *testing.T) {
	store := testutil.TestKVStore(t, "test")

	allocator, err := NewNumberAllocator(store)
	require.NoError(t, err, "new number allocator")

	actorID := abi.ActorID(10086)

	// normal
	for i := 0; i < 10; i++ {
		next, ok, err := allocator.Next(context.Background(), actorID, 0, func(_ uint64) bool { return true })
		require.NoErrorf(t, err, "allocate sector number, round %d", i)
		require.Truef(t, ok, "sector number allocated, round %d", i)
		require.Equal(t, uint64(i+1), next, "allocated sector number")
	}

	// reset min number every round
	var last uint64
	for ratio := 2; ratio <= 10; ratio++ {
		min := uint64(ratio * 10)
		next, ok, err := allocator.Next(context.Background(), actorID, min, func(_ uint64) bool { return true })
		require.NoErrorf(t, err, "allocate sector number, min %d", min)
		require.Truef(t, ok, "sector number allocated, min %d", min)
		require.Equalf(t, min+1, next, "allocated sector number with min %d", min)
		last = next
	}

	// with min untouched
	for i := 0; i < 10; i++ {
		next, ok, err := allocator.Next(context.Background(), actorID, 0, func(_ uint64) bool { return true })
		require.NoErrorf(t, err, "allocate sector number, last %d, round %d", last, i)
		require.Truef(t, ok, "sector number allocated, last %d, round %d", last, i)
		require.Equal(t, uint64(i)+last+1, next, "allocated sector number with last %d, round %d", last, i)
	}

	// with max limit
	_, ok, err := allocator.Next(context.Background(), actorID, 0, func(n uint64) bool { return n < last })
	require.NoError(t, err, "allocate sector number with max limit")
	require.False(t, ok, "sector number should be limited")
}

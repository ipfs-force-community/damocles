package sectors

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/damocles/damocles-manager/testutil"
)

func TestAllocatorNextN(t *testing.T) {
	actorID := abi.ActorID(10086)

	for _, n := range []uint32{1, 2, 3, 4, 5} {
		store := testutil.BadgerKVStore(t, fmt.Sprintf("test_%d", n))
		allocator, err := NewNumberAllocator(store)
		require.NoError(t, err, "new number allocator")

		// normal
		for i := 0; i < 10; i++ {
			next, ok, err := allocator.NextN(context.Background(), actorID, n, 0, func(_ uint64) bool { return true })
			require.NoErrorf(t, err, "allocate sector number, round %d, n %d", i, n)
			require.Truef(t, ok, "sector number allocated, round %d, n %d", i, n)
			require.Equal(t, uint64(i+1)*uint64(n), next, "allocated sector number, round %d, n %d", i, n)
		}

		// reset min number every round
		var last uint64
		for ratio := 2; ratio <= 10; ratio++ {
			min := uint64(ratio * 100)
			next, ok, err := allocator.NextN(context.Background(), actorID, n, min, func(_ uint64) bool { return true })
			require.NoErrorf(t, err, "allocate sector number, min %d", min)
			require.Truef(t, ok, "sector number allocated, min %d", min)
			require.Equalf(t, min+uint64(n), next, "allocated sector number with min %d", min)
			last = next
		}

		// with min untouched
		for i := 0; i < 10; i++ {
			next, ok, err := allocator.NextN(context.Background(), actorID, n, 0, func(_ uint64) bool { return true })
			require.NoErrorf(t, err, "allocate sector number, last %d, round %d", last, i)
			require.Truef(t, ok, "sector number allocated, last %d, round %d", last, i)
			require.Equal(
				t,
				uint64(i+1)*uint64(n)+last,
				next,
				"allocated sector number with last %d, round %d",
				last,
				i,
			)
		}

		// with max limit
		_, ok, err := allocator.NextN(context.Background(), actorID, n, 0, func(x uint64) bool { return x < last })
		require.NoError(t, err, "allocate sector number with max limit")
		require.False(t, ok, "sector number should be limited")
	}
}

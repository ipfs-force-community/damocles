package objstore

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil"
	"github.com/stretchr/testify/require"
)

func (m *StoreManager) resetReserved(ctx context.Context) error {
	err := m.metadb.Del(ctx, storeReserveSummaryKey)
	if err != nil {
		return fmt.Errorf("reset store reserve summary: %w", err)
	}

	return nil
}

func TestStoreManagerReserverSpace(t *testing.T) {
	kvs, kvstop := testutil.TestKVStore(t)
	defer kvstop()

	storeName4K := "store-4K"
	storeName1M := "store-1M"
	storeNameReadOnly := "store-readonly"

	store4K, err := NewMockStore(Config{
		CompactConfig: CompactConfig{
			Name: storeName4K,
		},
	}, 4<<10)
	require.NoError(t, err, "construct store-4K")

	store1M, err := NewMockStore(Config{
		CompactConfig: CompactConfig{
			Name: storeName1M,
		},
	}, 1<<20)
	require.NoError(t, err, "construct store-1M")

	storeRO, err := NewMockStore(Config{
		CompactConfig: CompactConfig{
			Name: storeNameReadOnly,
		},
		ReadOnly: true,
	}, 1<<30)
	require.NoError(t, err, "construct store-RO")

	mgr, err := NewStoreManager([]Store{store4K, store1M, storeRO}, kvs)
	require.NoError(t, err, "construct store mgr")

	// selection
	{
		attempts := 256
		count4K := 0
		for i := 0; i < attempts; i++ {
			choice, err := mgr.ReserveSpace(context.Background(), strconv.Itoa(i), 1, nil)
			require.NoErrorf(t, err, "selection cases: reserve space for %d", i)
			require.NotNilf(t, choice, "selection cases: should not get nil for %d", i)
			require.NotEqualf(t, storeNameReadOnly, choice.Name, "selection cases: ro store should not be selected for %d", i)
			if choice.Name == storeName4K {
				count4K++
			}

		}

		if count1M := attempts - count4K; count4K == 0 || count1M == 0 || count4K/count1M >= 2 || count1M/count4K >= 2 {
			t.Logf("unexpected distribution: %d : %d", count4K, count1M)
		}

		err := mgr.resetReserved(context.Background())
		require.NoError(t, err, "reset reserved")
	}

	// single selection
	{

		type reserveRequest struct {
			name       string
			size       uint64
			candidates []string
		}

		cases := []struct {
			cause    string
			reqs     []reserveRequest
			expected *string
		}{
			{
				cause: "prefer 4k",
				reqs: []reserveRequest{
					{
						name:       "1",
						size:       1,
						candidates: []string{storeName4K},
					},
				},
				expected: &storeName4K,
			},
			{
				cause: "prefer 1M",
				reqs: []reserveRequest{
					{
						name:       "1",
						size:       1,
						candidates: []string{storeName1M},
					},
				},
				expected: &storeName1M,
			},
			{
				cause: "> 4k",
				reqs: []reserveRequest{
					{
						name:       "1",
						size:       4<<10 + 1,
						candidates: nil,
					},
				},
				expected: &storeName1M,
			},
			{
				cause: "too large",
				reqs: []reserveRequest{
					{
						name:       "1",
						size:       2 << 20,
						candidates: nil,
					},
				},
				expected: nil,
			},

			{
				cause: "4K not enough",
				reqs: []reserveRequest{
					{
						name:       "1",
						size:       4 << 10,
						candidates: []string{storeName4K},
					},
					{
						name:       "2",
						size:       1,
						candidates: nil,
					},
				},
				expected: &storeName1M,
			},
		}

		for _, c := range cases {
			err := mgr.resetReserved(context.Background())
			require.NoError(t, err, "reset reserved")

			reqCount := len(c.reqs)
			for ri := 0; ri < reqCount-1; ri++ {
				req := c.reqs[ri]
				_, err := mgr.ReserveSpace(context.Background(), req.name, req.size, req.candidates)
				require.NoErrorf(t, err, "cause: %s, step: #%d, req: %s, size: %d", c.cause, ri, req.name, req.size)
			}

			last := c.reqs[reqCount-1]
			choice, err := mgr.ReserveSpace(context.Background(), last.name, last.size, last.candidates)
			require.NoErrorf(t, err, "cause: %s, last step, req: %s, size: %d", c.cause, last.name, last.size)

			if c.expected == nil {
				require.Nilf(t, choice, "expected nil choice for %s", c.cause)
			} else {
				require.NotNilf(t, choice, "expected non-nil choice for %s", c.cause)
				require.Equalf(t, *c.expected, choice.Name, "expected chosen instance for %s", c.cause)
			}
		}
	}

}

func TestStoreManagerReserverSpaceWeighed(t *testing.T) {
	kvs, kvstop := testutil.TestKVStore(t)
	defer kvstop()

	storeName1 := "store-1"
	storeName1K := "store-1000"
	storeNameReadOnly := "store-readonly"

	store1, err := NewMockStore(Config{
		CompactConfig: CompactConfig{
			Name: storeName1,
		},
		Weight: 1,
	}, 1<<20)
	require.NoError(t, err, "construct store-1")

	store1K, err := NewMockStore(Config{
		CompactConfig: CompactConfig{
			Name: storeName1K,
		},
		Weight: 1000,
	}, 1<<20)
	require.NoError(t, err, "construct store-1K")

	storeRO, err := NewMockStore(Config{
		CompactConfig: CompactConfig{
			Name: storeNameReadOnly,
		},
		ReadOnly: true,
	}, 1<<30)
	require.NoError(t, err, "construct store-RO")

	mgr, err := NewStoreManager([]Store{store1, store1K, storeRO}, kvs)
	require.NoError(t, err, "construct store mgr")

	// selection
	{
		attempts := 1024
		count1 := 0
		for i := 0; i < attempts; i++ {
			choice, err := mgr.ReserveSpace(context.Background(), strconv.Itoa(i), 1, nil)
			require.NoErrorf(t, err, "selection cases: reserve space for %d", i)
			require.NotNilf(t, choice, "selection cases: should not get nil for %d", i)
			require.NotEqualf(t, storeNameReadOnly, choice.Name, "selection cases: ro store should not be selected for %d", i)
			if choice.Name == storeName1 {
				count1++
			}

		}

		require.Truef(t, count1 <= 20, "store-1 selected %d/%d times", count1, attempts)

		err := mgr.resetReserved(context.Background())
		require.NoError(t, err, "reset reserved")
	}
}

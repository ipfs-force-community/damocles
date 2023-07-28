package objstore

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/testutil"
	"github.com/stretchr/testify/require"
)

var (
	TRUE      = true
	THOUNSAND = uint(1000)
)

func (m *StoreManager) resetReserved(ctx context.Context) error {
	err := m.metadb.Del(ctx, storeReserveSummaryKey)
	if err != nil {
		return fmt.Errorf("reset store reserve summary: %w", err)
	}

	return nil
}

func TestStoreManagerReserverSpace(t *testing.T) {
	kvs := testutil.BadgerKVStore(t, "test")

	storeName4K := "store-4K"
	storeName1M := "store-1M"
	storeNameReadOnly := "store-readonly"

	store4K, err := NewMockStore(Config{
		Name: storeName4K,
	}, 4<<10)
	require.NoError(t, err, "construct store-4K")

	store1M, err := NewMockStore(Config{
		Name: storeName1M,
	}, 1<<20)
	require.NoError(t, err, "construct store-1M")

	storeRO, err := NewMockStore(Config{
		Name:     storeNameReadOnly,
		ReadOnly: &TRUE,
	}, 1<<30)
	require.NoError(t, err, "construct store-RO")

	mgr, err := NewStoreManager([]Store{store4K, store1M, storeRO}, nil, kvs)
	require.NoError(t, err, "construct store mgr")

	// selection
	{
		attempts := 256
		count4K := 0
		for i := 0; i < attempts; i++ {
			choice, err := mgr.ReserveSpace(context.Background(), abi.SectorID{
				Miner:  1,
				Number: abi.SectorNumber(i),
			}, 1, nil)
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
			num        abi.SectorNumber
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
						num:        1,
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
						num:        1,
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
						num:        1,
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
						num:        1,
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
						num:        1,
						size:       4 << 10,
						candidates: []string{storeName4K},
					},
					{
						num:        2,
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
				_, err := mgr.ReserveSpace(context.Background(), abi.SectorID{
					Miner:  1,
					Number: req.num,
				}, req.size, req.candidates)
				require.NoErrorf(t, err, "cause: %s, step: #%d, req: %d, size: %d", c.cause, ri, req.num, req.size)
			}

			last := c.reqs[reqCount-1]
			choice, err := mgr.ReserveSpace(context.Background(), abi.SectorID{
				Miner:  1,
				Number: last.num,
			}, last.size, last.candidates)
			require.NoErrorf(t, err, "cause: %s, last step, req: %d, size: %d", c.cause, last.num, last.size)

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
	kvs := testutil.BadgerKVStore(t, "test")

	storeName1 := "store-1"
	storeName1K := "store-1000"
	storeNameReadOnly := "store-readonly"

	store1, err := NewMockStore(Config{
		Name: storeName1,
	}, 1<<20)
	require.NoError(t, err, "construct store-1")

	store1K, err := NewMockStore(Config{
		Name:   storeName1K,
		Weight: &THOUNSAND,
	}, 1<<20)
	require.NoError(t, err, "construct store-1K")

	storeRO, err := NewMockStore(Config{
		Name:     storeNameReadOnly,
		ReadOnly: &TRUE,
	}, 1<<30)
	require.NoError(t, err, "construct store-RO")

	mgr, err := NewStoreManager([]Store{store1, store1K, storeRO}, nil, kvs)
	require.NoError(t, err, "construct store mgr")

	// selection
	{
		attempts := 1024
		count1 := 0
		for i := 0; i < attempts; i++ {
			choice, err := mgr.ReserveSpace(context.Background(), abi.SectorID{
				Miner:  1,
				Number: abi.SectorNumber(i),
			}, 1, nil)
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

func TestStoreSelectPolicy(t *testing.T) {
	cases := []struct {
		policy StoreSelectPolicy
		miner  abi.ActorID
		allow  bool
	}{
		// nils
		{
			policy: StoreSelectPolicy{},
			miner:  1,
			allow:  true,
		},

		// emptys
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{},
				DenyMiners:  []abi.ActorID{},
			},
			miner: 2,
			allow: true,
		},

		// allowed by whitelist
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{2},
			},
			miner: 2,
			allow: true,
		},

		// denied by whitelist
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{2},
			},
			miner: 3,
			allow: false,
		},

		// denied by blacklist
		{
			policy: StoreSelectPolicy{
				DenyMiners: []abi.ActorID{2},
			},
			miner: 2,
			allow: false,
		},

		// allowed as not in blacklist
		{
			policy: StoreSelectPolicy{
				DenyMiners: []abi.ActorID{2},
			},
			miner: 3,
			allow: true,
		},

		// both set case1
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{2},
				DenyMiners:  []abi.ActorID{3},
			},
			miner: 2,
			allow: true,
		},

		// both set case2
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{2},
				DenyMiners:  []abi.ActorID{3},
			},
			miner: 3,
			allow: false,
		},

		// both set case3
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{2},
				DenyMiners:  []abi.ActorID{3},
			},
			miner: 4,
			allow: false,
		},

		// blacklist has the priority
		{
			policy: StoreSelectPolicy{
				AllowMiners: []abi.ActorID{2},
				DenyMiners:  []abi.ActorID{2, 3},
			},
			miner: 2,
			allow: false,
		},
	}

	for i := range cases {
		c := cases[i]
		got := c.policy.Allowed(c.miner)
		require.Equalf(t, c.allow, got, "#%d policy checks for %d with allow: %v, deny: %v", i, c.miner, c.policy.AllowMiners, c.policy.DenyMiners)
	}
}

package sectors

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil/testmodules"
)

type mockMinerInfoAPI struct {
	infos map[abi.ActorID]*core.MinerInfo
}

func (m *mockMinerInfoAPI) Get(ctx context.Context, mid abi.ActorID) (*core.MinerInfo, error) {
	minfo, ok := m.infos[mid]
	if !ok {
		return nil, fmt.Errorf("not found")
	}

	return minfo, nil
}

func TestRebuildManager(t *testing.T) {
	scfg, _ := testmodules.MockSafeConfig(10, nil)
	kvstore, stop := testutil.TestKVStore(t)
	defer stop()

	minfos := map[abi.ActorID]*core.MinerInfo{}
	for mi := range scfg.Miners {
		minfo := &core.MinerInfo{
			ID:                  scfg.Miners[mi].Actor,
			SectorSize:          32 << 30,
			WindowPoStProofType: abi.RegisteredPoStProof_StackedDrgWindow32GiBV1,
			SealProofType:       abi.RegisteredSealProof_StackedDrg32GiBV1_1,
		}
		minfo.Addr, _ = address.NewIDAddress(uint64(minfo.ID))

		minfos[minfo.ID] = minfo
	}

	rbmgr, err := NewRebuildManager(scfg, &mockMinerInfoAPI{
		infos: minfos,
	}, kvstore)

	require.NoError(t, err, "construct rebuild manager")

	// 获取 scfg 中不存在的 actors
	{
		allocated, err := rbmgr.Allocate(context.Background(), core.AllocateSectorSpec{})

		require.NoError(t, err, "allocate for empty infos")
		require.Nil(t, allocated, "non-nil rebuild info for empty infos")
	}

	for i := 0; i < 12; i++ {
		for num := 0; num < 16; num++ {
			sid := abi.SectorID{
				Miner:  testmodules.TestActorBase + abi.ActorID(i),
				Number: abi.SectorNumber(num),
			}

			err := rbmgr.Set(context.Background(), sid, core.SectorRebuildInfo{
				Sector: core.AllocatedSector{
					ID:        sid,
					ProofType: abi.RegisteredSealProof_StackedDrg32GiBV1,
				},
			})

			require.NoErrorf(t, err, "set rebuild info for %d %d", sid.Miner, sid.Number)
		}
	}

	err = rbmgr.loadAndUpdate(context.Background(), func(infos *RebuildInfos) bool {
		require.Len(t, infos.Actors, 12, "actors count")
		for ai := range infos.Actors {
			require.Lenf(t, infos.Actors[ai].Infos, 16, "rebuild info count for actor %d", infos.Actors[ai].Actor)
		}
		return false
	})

	require.NoError(t, err, "check rebuild info count after init")

	// 获取 scfg 中不存在的 actors
	{
		allocated, err := rbmgr.Allocate(context.Background(), core.AllocateSectorSpec{
			AllowedMiners: []abi.ActorID{testmodules.TestActorBase + 10, testmodules.TestActorBase + 11},
		})

		require.NoError(t, err, "allocate for non-exist actors")
		require.Nil(t, allocated, "non-nil rebuild info for non-exist actors")
	}

	// 获取 scfg 中不存在的 abi.RegisteredSealProof
	{
		allocated, err := rbmgr.Allocate(context.Background(), core.AllocateSectorSpec{
			AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg64GiBV1},
		})

		require.NoError(t, err, "allocate for non-exist RegisteredSealProof")
		require.Nil(t, allocated, "non-nil rebuild info for non-exist RegisteredSealProof")
	}

	allocatedCount := map[abi.ActorID]int{}

	// 正常分配
	{
		allocated, err := rbmgr.Allocate(context.Background(), core.AllocateSectorSpec{})

		require.NoError(t, err, "allocate for normal allocation")
		require.NotNil(t, allocated, "nil rebuild info for normal allocation")
		allocatedCount[allocated.Sector.ID.Miner] = allocatedCount[allocated.Sector.ID.Miner] + 1
	}

	// 按 abi.Actor 过滤
	{
		allocated, err := rbmgr.Allocate(context.Background(), core.AllocateSectorSpec{
			AllowedMiners: []abi.ActorID{testmodules.TestActorBase + 1},
		})

		require.NoError(t, err, "allocate for allocation based on miners filter")
		require.NotNil(t, allocated, "nil rebuild info for allocation based on miners filter")
		allocatedCount[allocated.Sector.ID.Miner] = allocatedCount[allocated.Sector.ID.Miner] + 1
	}

	// 按 abi.RegisteredSealProof 过滤
	{
		allocated, err := rbmgr.Allocate(context.Background(), core.AllocateSectorSpec{
			AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg32GiBV1_1},
		})

		require.NoError(t, err, "allocate for empty infos")
		require.NotNil(t, allocated, "nil rebuild info for allocation based on proof types filter")
		allocatedCount[allocated.Sector.ID.Miner] = allocatedCount[allocated.Sector.ID.Miner] + 1
	}

	err = rbmgr.loadAndUpdate(context.Background(), func(infos *RebuildInfos) bool {
		require.Len(t, infos.Actors, 12, "actors count")
		for ai := range infos.Actors {
			allocated := allocatedCount[infos.Actors[ai].Actor]
			require.Lenf(t, infos.Actors[ai].Infos, 16-allocated, "rebuild info count for actor %d with %d allocated", infos.Actors[ai].Actor, allocated)
		}
		return false
	})

	require.NoError(t, err, "check rebuild info count after allocation tests")
}

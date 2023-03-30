package sectors

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil/testmodules"
)

func TestUnsealManager(t *testing.T) {
	ctx := context.Background()
	scfg, _ := testmodules.MockSafeConfig(3, nil)
	kvstore := testutil.TestKVStore(t, "test_unseal")

	marketEvent := market.NewMockIMarketEvent(gomock.NewController(t))

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

	marketEvent.EXPECT().OnUnseal(gomock.Any()).AnyTimes()
	umgr, err := NewUnsealManager(ctx, scfg, &mockMinerInfoAPI{
		infos: minfos,
	}, kvstore, marketEvent)
	require.NoError(t, err, "construct unseal manager")

	sectorUnsealInfoCase := []core.SectorUnsealInfo{}
	for mi := range scfg.Miners {
		for i := 0; i < 3; i++ {
			require.NoError(t, err, "new random uuid")
			sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
				Id: uuid.New(),
				SectorID: abi.SectorID{
					Miner:  abi.ActorID(scfg.Miners[mi].Actor),
					Number: abi.SectorNumber(i),
				},
				Offset: 0,
				Size:   32 << 30,
			})
		}
	}

	t.Run("set and allocate unseal task", func(t *testing.T) {
		for _, v := range sectorUnsealInfoCase {
			err := umgr.Set(context.Background(), &v)
			require.NoError(t, err, "set unseal task")

			allocated, err := umgr.Allocate(context.Background(), core.AllocateSectorSpec{
				AllowedMiners:     []abi.ActorID{v.SectorID.Miner},
				AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg32GiBV1_1},
			})
			require.NoError(t, err, "allocate unseal task no err")
			require.NotNil(t, allocated, "allocate unseal task not nil")
			require.Equal(t, v, *allocated, "allocate unseal task equal the one set before")
		}

		var infos *UnsealInfos
		err := umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")

		require.Equal(t, len(sectorUnsealInfoCase), len(infos.Allocated), "all task has been allocated")
		for _, v := range infos.Allocatable {
			require.Equal(t, 0, len(v), "no allocatable task left")
		}
	})

	// todo: test archive
}

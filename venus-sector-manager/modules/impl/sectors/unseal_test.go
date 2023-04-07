package sectors

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	vtypes "github.com/filecoin-project/venus/venus-shared/types"
	"github.com/golang/mock/gomock"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/testutil/testmodules"
)

func TestUnsealManager(t *testing.T) {
	ctx := context.Background()
	scfg, _ := testmodules.MockSafeConfig(3, nil)
	scfg.Config.Common.API.Market = "/dns/market/tcp/41235"

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

	t.Run("set and allocate unseal task", func(t *testing.T) {
		kvstore := testutil.TestKVStore(t, "test_unseal_01")
		marketEvent := market.NewMockIMarketEvent(gomock.NewController(t))
		marketEvent.EXPECT().OnUnseal(gomock.Any())
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerInfoAPI{
			infos: minfos,
		}, kvstore, marketEvent)
		require.NoError(t, err, "construct unseal manager")
		sectorUnsealInfoCase := []core.SectorUnsealInfo{}
		for mi := range scfg.Miners {
			for i := 0; i < 3; i++ {
				sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
					EventIds: []vtypes.UUID{vtypes.NewUUID()},
					UnsealTaskIdentifier: core.UnsealTaskIdentifier{
						Actor:        scfg.Miners[mi].Actor,
						SectorNumber: abi.SectorNumber(i),
					},
					Offset: 0,
					Size:   32 << 30,
					Dest:   "https://host/path",
				})
			}
		}

		for i, v := range sectorUnsealInfoCase {
			err := umgr.Set(context.Background(), &sectorUnsealInfoCase[i])
			require.NoError(t, err, "set unseal task")

			allocated, err := umgr.Allocate(context.Background(), core.AllocateSectorSpec{
				AllowedMiners:     []abi.ActorID{v.Actor},
				AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg32GiBV1_1},
			})
			require.NoError(t, err, "allocate unseal task no err")
			require.NotNil(t, allocated, "allocate unseal task not nil")
			require.Equal(t, v, *allocated, "allocate unseal task equal the one set before")
		}

		var infos *UnsealInfos
		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")

		require.Equal(t, len(sectorUnsealInfoCase), len(infos.Allocated), "all task has been allocated")
		for _, v := range infos.Allocatable {
			require.Equal(t, 0, len(v), "no allocatable task left")
		}

		for _, v := range infos.Allocated {
			for _, eventID := range v.EventIds {
				marketEvent.EXPECT().RespondUnseal(gomock.Any(), eventID, gomock.Any())
			}
			err := umgr.Archive(ctx, &v.UnsealTaskIdentifier, nil)
			require.NoError(t, err, "archive unseal task")
		}

		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")
		require.Equal(t, 0, len(infos.Allocated), "all task has been archived")
	})

	t.Run("set and allocate unseal task with multiple request event id", func(t *testing.T) {
		kvstore := testutil.TestKVStore(t, "test_unseal_02")
		marketEvent := market.NewMockIMarketEvent(gomock.NewController(t))
		marketEvent.EXPECT().OnUnseal(gomock.Any())
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerInfoAPI{
			infos: minfos,
		}, kvstore, marketEvent)
		require.NoError(t, err, "construct unseal manager")
		sectorUnsealInfoCase := []core.SectorUnsealInfo{}
		pCid, err := cid.V0Builder.Sum(cid.V0Builder{}, []byte("piece"))
		require.NoError(t, err, "new piece cid")

		for i := 0; i < 3; i++ {
			sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
				EventIds: []vtypes.UUID{vtypes.NewUUID()},
				UnsealTaskIdentifier: core.UnsealTaskIdentifier{
					Actor:        scfg.Miners[0].Actor,
					SectorNumber: abi.SectorNumber(0),
					PieceCid:     pCid,
				},
				Offset: 0,
				Size:   32 << 30,
				Dest:   "https://host/path",
			})
		}

		for i := range sectorUnsealInfoCase {
			err := umgr.Set(context.Background(), &sectorUnsealInfoCase[i])
			require.NoError(t, err, "set unseal task")
		}

		var infos *UnsealInfos
		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")

		allocatableMap := infos.Allocatable[scfg.Miners[0].Actor]
		require.Equal(t, 1, len(allocatableMap), "only one allocatable task left")

		var allocatable *core.SectorUnsealInfo
		for _, v := range allocatableMap {
			allocatable = v
		}
		require.Equal(t, 3, len(allocatable.EventIds), "allocatable task has 3 event ids")

		for _, v := range infos.Allocated {
			for _, eventID := range v.EventIds {
				marketEvent.EXPECT().RespondUnseal(gomock.Any(), eventID, gomock.Any())
			}
			err := umgr.Archive(ctx, &v.UnsealTaskIdentifier, nil)
			require.NoError(t, err, "archive unseal task")
		}

		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")
		require.Equal(t, 0, len(infos.Allocated), "all task has been archived")
	})
}

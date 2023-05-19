package sectors

import (
	"context"
	"net/url"
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
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerAPI{
			infos: minfos,
		}, kvstore, marketEvent)
		require.NoError(t, err, "construct unseal manager")
		sectorUnsealInfoCase := []core.SectorUnsealInfo{}
		for mi := range scfg.Miners {
			for i := 0; i < 3; i++ {
				sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
					EventIds: []vtypes.UUID{vtypes.NewUUID()},
					Sector: core.AllocatedSector{
						ID: abi.SectorID{
							Miner:  scfg.Miners[mi].Actor,
							Number: abi.SectorNumber(i),
						},
						// proofType should be set when allocate
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
				AllowedMiners:     []abi.ActorID{v.Sector.ID.Miner},
				AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg32GiBV1_1},
			})
			require.NoError(t, err, "allocate unseal task no err")
			require.NotNil(t, allocated, "allocate unseal task not nil")
			v.Sector.ProofType = allocated.Sector.ProofType
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
			err := umgr.Achieve(ctx, v.Sector.ID, v.PieceCid, "")
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
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerAPI{
			infos: minfos,
		}, kvstore, marketEvent)
		require.NoError(t, err, "construct unseal manager")
		sectorUnsealInfoCase := []core.SectorUnsealInfo{}
		pCid, err := cid.V0Builder.Sum(cid.V0Builder{}, []byte("piece"))
		require.NoError(t, err, "new piece cid")

		for i := 0; i < 3; i++ {
			sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
				EventIds: []vtypes.UUID{vtypes.NewUUID()},
				Sector: core.AllocatedSector{
					ID: abi.SectorID{
						Miner:  scfg.Miners[0].Actor,
						Number: abi.SectorNumber(0),
					},
					// proofType should be set when allocate
				},
				PieceCid: pCid,
				Offset:   0,
				Size:     32 << 30,
				Dest:     "https://host/path",
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
			err := umgr.Achieve(ctx, v.Sector.ID, v.PieceCid, "")
			require.NoError(t, err, "achieve unseal task")
		}

		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")
		require.Equal(t, 0, len(infos.Allocated), "all task has been archived")
	})

	t.Run("set and allocate unseal task without event id", func(t *testing.T) {
		kvstore := testutil.TestKVStore(t, "test_unseal_01")
		marketEvent := market.NewMockIMarketEvent(gomock.NewController(t))
		marketEvent.EXPECT().OnUnseal(gomock.Any())
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerAPI{
			infos: minfos,
		}, kvstore, marketEvent)
		require.NoError(t, err, "construct unseal manager")

		sectorUnsealInfo := &core.SectorUnsealInfo{
			Sector: core.AllocatedSector{
				ID: abi.SectorID{
					Miner:  scfg.Miners[0].Actor,
					Number: abi.SectorNumber(0),
				},
				// proofType should be set when allocate
			},
			Offset: 0,
			Size:   32 << 30,
			Dest:   "https://host/path",
		}
		err = umgr.Set(context.Background(), sectorUnsealInfo)
		require.NoError(t, err, "set unseal task")

		allocated, err := umgr.Allocate(context.Background(), core.AllocateSectorSpec{
			AllowedMiners:     []abi.ActorID{sectorUnsealInfo.Sector.ID.Miner},
			AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg32GiBV1_1},
		})
		require.NoError(t, err, "allocate unseal task no err")
		require.NotNil(t, allocated, "allocate unseal task not nil")
		sectorUnsealInfo.Sector.ProofType = allocated.Sector.ProofType
		require.Equal(t, *sectorUnsealInfo, *allocated, "allocate unseal task equal the one set before")

		var infos *UnsealInfos
		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")

		require.Equal(t, 1, len(infos.Allocated), "all task has been allocated")
		for _, v := range infos.Allocatable {
			require.Equal(t, 0, len(v), "no allocatable task left")
		}

		for _, v := range infos.Allocated {
			for _, eventID := range v.EventIds {
				marketEvent.EXPECT().RespondUnseal(gomock.Any(), eventID, gomock.Any())
			}
			err := umgr.Achieve(ctx, v.Sector.ID, v.PieceCid, "")
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

func TestCheckUrl(t *testing.T) {
	testCases := []struct {
		url    string
		expect string
	}{
		{
			url:    "https://host/path?any=any",
			expect: "https://host/path?any=any",
		},
		{
			url:    "http://host/path?any=any",
			expect: "http://host/path?any=any",
		},
		{
			url:    "file:///path/to/file",
			expect: "file:///path/to/file",
		},
		{
			url:    "market://store_name/piece_cid",
			expect: "market://store_name/piece_cid?host=market_ip&scheme=https&token=vsm_token",
		},
		{
			url:    "store://store_name/piece_cid?any=any",
			expect: "store://store_name/piece_cid?any=any",
		},
	}
	preSetURL, err := url.Parse("https://market_ip/?token=vsm_token")
	if err != nil {
		t.Fatal(err)
	}
	u := UnsealManager{
		defaultDest: preSetURL,
	}

	for _, v := range testCases {
		res, err := u.checkDestUrl(v.url)
		require.NoError(t, err)
		require.Equal(t, v.expect, res)
	}

	_, err = u.checkDestUrl("oss://bucket/path")
	require.Error(t, err)
}

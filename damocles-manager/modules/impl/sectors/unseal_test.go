package sectors

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/types/gateway"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/testutil"
	"github.com/ipfs-force-community/damocles/damocles-manager/testutil/testmodules"
)

func TestUnsealManager(t *testing.T) {
	ctx := context.Background()
	scfg, _ := testmodules.MockSafeConfig(3, nil)
	marketAddr := "/dns/market/tcp/41235"
	scfg.Config.Common.API.Market = &marketAddr

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

	t.Run("set, allocate, achieve unseal task", func(t *testing.T) {
		kvstore := testutil.BadgerKVStore(t, "test_unseal_01")
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerAPI{
			infos: minfos,
		}, kvstore, "/dns/market/tcp/41235", "")
		require.NoError(t, err, "construct unseal manager")
		sectorUnsealInfoCase := []core.SectorUnsealInfo{}
		for mi := range scfg.Miners {
			for i := 0; i < 3; i++ {
				sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
					Sector: core.AllocatedSector{
						ID: abi.SectorID{
							Miner:  scfg.Miners[mi].Actor,
							Number: abi.SectorNumber(i),
						},
						// proofType should be set when allocate
					},
					Offset: 0,
					Size:   32 << 30,
					Dest:   []string{"https://host/path"},
				})
			}
		}

		for i, v := range sectorUnsealInfoCase {
			_, err := umgr.SetAndCheck(context.Background(), &sectorUnsealInfoCase[i])
			require.NoError(t, err, "set unseal task")

			allocated, err := umgr.Allocate(context.Background(), core.AllocateSectorSpec{
				AllowedMiners:     []abi.ActorID{v.Sector.ID.Miner},
				AllowedProofTypes: []abi.RegisteredSealProof{abi.RegisteredSealProof_StackedDrg32GiBV1_1},
			})
			require.NoError(t, err, "allocate unseal task no err")
			require.NotNil(t, allocated, "allocate unseal task not nil")
			v.Sector.ProofType = allocated.Sector.ProofType
			v.State = gateway.UnsealStateUnsealing
			require.Equal(t, v, *allocated, "allocate unseal task equal the one set before")
		}

		var infos *UnsealInfos
		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")
		require.Equal(t, len(sectorUnsealInfoCase), len(infos.Data), "all task has been allocated")
		allocatable := 0
		for _, v := range infos.AllocIndex {
			allocatable += len(v)
		}
		require.Equal(t, 0, allocatable, "no allocatable task left")

		for _, v := range infos.Data {
			err := umgr.Achieve(ctx, v.Sector.ID, v.PieceCid, "")
			require.NoError(t, err, "archive unseal task")
		}

		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")
		achieveCount := 0
		for _, v := range infos.Data {
			if v.State == gateway.UnsealStateFinished {
				achieveCount++
			}
		}
		require.Equal(t, 9, achieveCount, "all task has been archived")

		for _, v := range infos.Data {
			state, err := umgr.SetAndCheck(ctx, v)
			require.NoError(t, err, "set unseal task again")
			require.Equal(t, gateway.UnsealStateFinished, state, "get state finished")
		}

		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			infos = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")
		require.Equal(t, 0, len(infos.Data), "all task has been done and view")
	})

	t.Run("set and allocate unseal task with multiple dest", func(t *testing.T) {
		kvstore := testutil.BadgerKVStore(t, "test_unseal_02")
		umgr, err := NewUnsealManager(ctx, scfg, &mockMinerAPI{
			infos: minfos,
		}, kvstore, "/dns/market/tcp/41235", "")
		require.NoError(t, err, "construct unseal manager")
		sectorUnsealInfoCase := []core.SectorUnsealInfo{}
		pCid, err := cid.V0Builder.Sum(cid.V0Builder{}, []byte("piece"))
		require.NoError(t, err, "new piece cid")

		for i := 0; i < 3; i++ {
			sectorUnsealInfoCase = append(sectorUnsealInfoCase, core.SectorUnsealInfo{
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
				Dest:     []string{fmt.Sprintf("https://host/path/%d", i)},
			})
		}

		for i := range sectorUnsealInfoCase {
			_, err := umgr.SetAndCheck(context.Background(), &sectorUnsealInfoCase[i])
			require.NoError(t, err, "set unseal task")
		}

		var db *UnsealInfos
		err = umgr.loadAndUpdate(ctx, func(i *UnsealInfos) bool {
			db = i
			return false
		})
		require.NoError(t, err, "loadAndUpdate unseal task")

		keySet := db.AllocIndex[scfg.Miners[0].Actor]
		require.Equal(t, 1, len(keySet), "only one allocatable task left")

		var allocatable *core.SectorUnsealInfo
		for k := range keySet {
			allocatable = db.Data[k]
		}
		require.Equal(t, 3, len(allocatable.Dest), "allocatable task has 3 destinations")
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

func TestMarketPieceStore(t *testing.T) {
	testCases := []struct {
		name       string
		marketAddr string
		expect     string
		hasErr     bool
	}{
		{
			name:       "empty address",
			marketAddr: "",
			expect:     "http://" + invalidMarketHost,
			hasErr:     true,
		},

		{
			name:       "normal address",
			marketAddr: "/dns/market/tcp/7890",
			expect:     "http://market:7890/resource",
			hasErr:     false,
		},
	}
	for _, v := range testCases {
		t.Run(v.name, func(t *testing.T) {
			url, err := getDefaultMarketPiecesStore(v.marketAddr)
			require.Equal(t, v.expect, url.String())
			if v.hasErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}

}

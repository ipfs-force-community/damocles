package poster

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/stretchr/testify/require"
)

var (
	invalidSender             = modules.MustAddress(address.Undef)
	testActorBase abi.ActorID = 10000
)

func mockSafeConfig(count int) (*modules.SafeConfig, sync.Locker) {
	cfg := modules.DefaultConfig(false)
	for i := 0; i < count; i++ {
		mcfg := modules.DefaultMinerConfig(false)
		mcfg.Actor = testActorBase + abi.ActorID(i)
		mcfg.PoSt.Enabled = true
		mcfg.PoSt.Sender = modules.GetFakeAddress()
		cfg.Miners = append(cfg.Miners, mcfg)
	}

	var lock sync.RWMutex

	return &modules.SafeConfig{
		Config: &cfg,
		Locker: lock.RLocker(),
	}, &lock
}

const blkRaw = `{"Miner":"t038057","Ticket":{"VRFProof":"kfggWR2GcEbfTuJ20hkAFNRbF7xusDuAQR7XwTjJ2/gc1rwIDmaXbSVxXe4j1njcCBoMhmlYIn9D/BLqQuIOayMHPYvDmOJGc9M27Hwg1UZkiuJmXji+iM/JBNYaOA61"},"ElectionProof":{"WinCount":1,"VRFProof":"tI7cWWM9sGsKc69N9DjN41glaO5Hg7r742H56FPzg7szbhTrxj8kw0OsiJzcPJdiAa6D5jZ1S2WKoLK7lwg2R5zYvCRwwWLGDiExqbqsvqmH5z/e6YGpaD7ghTPRH1SR"},"BeaconEntries":[{"Round":2118576,"Data":"rintMKcvVAslYpn9DcshDBmlPN6hUR+wjvVQSkkVUK5klx1kOSpcDvzODSc2wXFQA7BVbEcXJW/5KLoL0KHx2alLUWDOwxhsIQnAydXdZqG8G76nTIgogthfIMgSGdB2"}],"WinPoStProof":[{"PoStProof":3,"ProofBytes":"t0ZgPHFv0ao9fVZJ/fxbBrzATmOiIv9/IueSyAjtcqEpxqWViqchaaxnz1afwzAbhahpfZsGiGWyc408WYh7Q8u0Aa52KGPmUNtf3pAvxWfsUDMz9QUfhLZVg/p8/PUVC/O/E7RBNq4YPrRK5b6Q8PVwzIOxGOS14ge6ys8Htq+LfNJbcqY676qOYF4lzMuMtQIe3CxMSAEaEBfNpHhAEs83dO6vll9MZKzcXYpNWeqmMIz4xSdF18StQq9vL/Lo"}],"Parents":[{"/":"bafy2bzacecf4wtqz3kgumeowhdulejk3xbfzgibfyhs42x4vx2guqgudem2hg"},{"/":"bafy2bzacebkpxh2k63xreigl6a3ggdr2adwk67b4zw5dddckhqex2tmha6hee"},{"/":"bafy2bzacecor3xq4ykmhhrgq55rdo5w7up65elc4qwx5uwjy25ffynidskbxw"},{"/":"bafy2bzacedr2mztmef65fodqzvyjcdnsgpcjthstseinll4maqg24avnv7ljo"}],"ParentWeight":"21779626255","Height":1164251,"ParentStateRoot":{"/":"bafy2bzacecypgutbewmyop2wfuafvxt7dm7ew4u3ssy2p4rn457f6ynrj2i6a"},"ParentMessageReceipts":{"/":"bafy2bzaceaflsspsxuxew2y4g6o72wp5i2ewp3fcolga6n2plw3gycam7s4lg"},"Messages":{"/":"bafy2bzaceanux5ivzlxzvhqxtwc5vkktcfqepubwtwgv26dowzbl3rtgqk54k"},"BLSAggregate":{"Type":2,"Data":"lQg9jBfYhY2vvjB/RPlWg6i+MBTlH1u0lmdasiab5BigsKAuZSeLNlTGbdoVZhAsDUT59ZdGsMmueHjafygDUN2KLhZoChFf6LQHH42PTSXFlkRVHvmKVz9DDU03FLMB"},"Timestamp":1658988330,"BlockSig":{"Type":2,"Data":"rMOv2tXKqV5VDOq5IQ35cP0cCAzGmaugVr/g5JTrilhAn4LYK0h6ByPL5cX5ONzlDTx9+zYZFteIzaenirZhw7G510Lh0J8lbTLP5X2EX251rEA8dpkPZPcNylzN0r8X"},"ForkSignaling":0,"ParentBaseFee":"100"}`

func mockTipSet(t *testing.T, height abi.ChainEpoch) *types.TipSet {
	var blk types.BlockHeader
	err := json.Unmarshal([]byte(blkRaw), &blk)
	require.NoError(t, err, "unmarshal block header raw")
	blk.Height = height

	ts, err := types.NewTipSet([]*types.BlockHeader{&blk})
	require.NoError(t, err, "construct tipset")
	return ts
}

func TestPoSterGetEnabledMiners(t *testing.T) {
	minerCount := 128
	scfg, wmu := mockSafeConfig(minerCount)

	require.Len(t, scfg.Miners, minerCount, "mocked miners")

	poster, err := newPoSterWithRunnerConstructor(scfg, nil, nil, nil, nil, nil, nil, nil, mockRunnerConstructor(&mockRunner{}))
	require.NoError(t, err, "new poster")

	mids := poster.getEnabledMiners(logging.Nop)
	require.Len(t, mids, minerCount, "enabled miners")

	{
		wmu.Lock()
		disableCount := rand.Intn(minerCount)
		for i := 0; i < disableCount; i++ {
			scfg.Config.Miners[i].PoSt.Enabled = false
		}
		wmu.Unlock()

		mids := poster.getEnabledMiners(logging.Nop)
		require.Lenf(t, mids, minerCount-disableCount, "%d disabled", disableCount)

		wmu.Lock()
		for i := 0; i < disableCount; i++ {
			scfg.Config.Miners[i].PoSt.Enabled = true
		}
		wmu.Unlock()

		mids = poster.getEnabledMiners(logging.Nop)
		require.Lenf(t, mids, minerCount, "reset after %d disabled", disableCount)
	}

	{
		wmu.Lock()
		invalidCount := rand.Intn(minerCount)
		for i := 0; i < invalidCount; i++ {
			scfg.Config.Miners[i].PoSt.Sender = invalidSender
		}
		wmu.Unlock()

		mids := poster.getEnabledMiners(logging.Nop)
		require.Lenf(t, mids, minerCount-invalidCount, "%d invalid address", invalidCount)

		wmu.Lock()
		for i := 0; i < invalidCount; i++ {
			scfg.Config.Miners[i].PoSt.Sender = modules.GetFakeAddress()
		}
		wmu.Unlock()

		mids = poster.getEnabledMiners(logging.Nop)
		require.Lenf(t, mids, minerCount, "reset after %d invalid", invalidCount)
	}
}

func TestFetchMinerProvingDeadlineInfos(t *testing.T) {
	minerCount := 16
	scfg, _ := mockSafeConfig(minerCount)
	require.Len(t, scfg.Miners, minerCount, "mocked miners")

	mids := make([]abi.ActorID, len(scfg.Miners))
	for i := range scfg.Miners {
		mids[i] = scfg.Miners[i].Actor
	}

	dl := randomDeadline()
	var mockChain chain.MockStruct
	mockChain.IMinerStateStruct.Internal.StateMinerProvingDeadline = func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
		return dl, nil
	}

	poster, err := newPoSterWithRunnerConstructor(scfg, &mockChain, nil, nil, nil, nil, nil, nil, mockRunnerConstructor(&mockRunner{}))
	require.NoError(t, err, "new poster")

	ts := mockTipSet(t, 1000)

	// 默认行为
	{

		dinfos := poster.fetchMinerProvingDeadlineInfos(context.Background(), mids, ts)
		require.Len(t, dinfos, len(mids), "get all dlines for miners")
	}

	// 禁用部分
	{
		for i := 0; i < 8; i++ {
			available := rand.Intn(minerCount)
			mockChain.IMinerStateStruct.Internal.StateMinerProvingDeadline = func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
				id, err := address.IDFromAddress(p1)
				if err != nil {
					return nil, err
				}

				if id-uint64(testActorBase) >= uint64(available) {
					return nil, fmt.Errorf("not available")
				}

				return dl, nil
			}

			dinfos := poster.fetchMinerProvingDeadlineInfos(context.Background(), mids, ts)
			require.Lenf(t, dinfos, available, "get only available dlines for miners")
		}
	}

}

func TestHandleHeadChange(t *testing.T) {
	minerCount := 16
	scfg, _ := mockSafeConfig(minerCount)
	require.Len(t, scfg.Miners, minerCount, "mocked miners")

	mids := make([]abi.ActorID, len(scfg.Miners))
	for i := range scfg.Miners {
		mids[i] = scfg.Miners[i].Actor
	}

	var mockChain chain.MockStruct
	mockChain.IMinerStateStruct.Internal.StateMinerInfo = func(ctx context.Context, maddr address.Address, tsk types.TipSetKey) (types.MinerInfo, error) {
		_, err := address.IDFromAddress(maddr)
		if err != nil {
			return types.MinerInfo{}, err
		}

		return types.MinerInfo{
			WindowPoStProofType: abi.RegisteredPoStProof_StackedDrgWindow2KiBV1,
			SectorSize:          abi.SectorSize(2 << 10),
		}, nil
	}

	countRunners := func(p *PoSter) int {
		var count int
		for _, m := range p.schedulers {
			count += len(m)
		}

		return count
	}

	ctx := context.Background()

	// 默认的启动和清除
	{
		runner := &mockRunner{}

		dl := deadlineAt(10000, 0)
		mockChain.IMinerStateStruct.Internal.StateMinerProvingDeadline = func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
			return dl, nil
		}

		poster, err := newPoSterWithRunnerConstructor(scfg, &mockChain, nil, nil, chain.NewMinerInfoAPI(&mockChain), nil, nil, nil, mockRunnerConstructor(runner))
		require.NoError(t, err, "new poster")

		cases := []struct {
			title       string
			height      abi.ChainEpoch
			runnerCount int
			started     uint32
			submited    uint32
			aborted     uint32
		}{
			{
				title:       "before challenge",
				height:      dl.Challenge - 1,
				runnerCount: minerCount,
				started:     0,
				submited:    0,
				aborted:     0,
			},

			{
				title:       "challenge",
				height:      dl.Challenge,
				runnerCount: minerCount,
				started:     0,
				submited:    0,
				aborted:     0,
			},

			{
				title:       "challenge + condidence",
				height:      dl.Challenge + abi.ChainEpoch(DefaultChallengeConfidence),
				runnerCount: minerCount,
				started:     uint32(minerCount),
				submited:    0,
				aborted:     0,
			},

			{
				title:       "submit",
				height:      dl.Open + abi.ChainEpoch(DefaultSubmitConfidence),
				runnerCount: minerCount,
				started:     uint32(minerCount * 2),
				submited:    uint32(minerCount),
				aborted:     0,
			},

			{
				title:       "close",
				height:      dl.Close,
				runnerCount: 0,
				started:     uint32(minerCount * 2),
				submited:    uint32(minerCount * 1),
				aborted:     uint32(minerCount),
			},
		}

		for _, c := range cases {
			ts := mockTipSet(t, c.height)
			dinfos := poster.fetchMinerProvingDeadlineInfos(ctx, mids, ts)
			require.Len(t, dinfos, minerCount, "dlines for all mienrs", c.title, c.height)

			poster.handleHeadChange(ctx, nil, ts, dinfos)

			require.Equal(t, c.runnerCount, countRunners(poster), "runner count", c.title, c.height)
			require.Equal(t, c.started, runner.started, "started runners", c.title, c.height)
			require.Equal(t, c.submited, runner.submited, "submited runners", c.title, c.height)
			require.Equal(t, c.aborted, runner.aborted, "aborted runners", c.title, c.height)
		}

	}

	// 连续 deadlines
	{
		periodStart := abi.ChainEpoch(10000)
		dls := []*dline.Info{
			deadlineAt(periodStart, 0),
			deadlineAt(periodStart, 1),
			deadlineAt(periodStart, 2),
			deadlineAt(periodStart, 3),
		}

		called := uint32(0)

		mockChain.IMinerStateStruct.Internal.StateMinerProvingDeadline = func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
			index := (atomic.AddUint32(&called, 1) - 1) / uint32(minerCount)
			if int(index) >= len(dls) {
				return nil, fmt.Errorf("invalid index %d", index)
			}

			return dls[index], nil
		}

		runner := &mockRunner{}
		poster, err := newPoSterWithRunnerConstructor(scfg, &mockChain, nil, nil, chain.NewMinerInfoAPI(&mockChain), nil, nil, nil, mockRunnerConstructor(runner))
		require.NoError(t, err, "new poster")

		for di := range dls {
			dl := dls[di]

			ts := mockTipSet(t, dl.Challenge)

			dinfos := poster.fetchMinerProvingDeadlineInfos(ctx, mids, ts)
			require.Len(t, dinfos, minerCount, "dlines for all mienrs", dl.Challenge)

			poster.handleHeadChange(ctx, nil, ts, dinfos)

			if di == 0 {
				require.Equal(t, minerCount, countRunners(poster), "runner count for dl index", dl.Index)
			} else {
				require.Equal(t, minerCount*2, countRunners(poster), "runner count for dl index", dl.Index)
			}
		}
	}

	// revert
	{
		runner := &mockRunner{}

		dl := deadlineAt(10000, 0)
		mockChain.IMinerStateStruct.Internal.StateMinerProvingDeadline = func(p0 context.Context, p1 address.Address, p2 types.TipSetKey) (*dline.Info, error) {
			return dl, nil
		}

		poster, err := newPoSterWithRunnerConstructor(scfg, &mockChain, nil, nil, chain.NewMinerInfoAPI(&mockChain), nil, nil, nil, mockRunnerConstructor(runner))
		require.NoError(t, err, "new poster")

		ts := mockTipSet(t, dl.Open)
		dinfos := poster.fetchMinerProvingDeadlineInfos(ctx, mids, ts)
		require.Len(t, dinfos, minerCount, "dlines for all mienrs")

		poster.handleHeadChange(ctx, nil, ts, dinfos)
		require.Equal(t, minerCount, countRunners(poster), "runner count")

		dinfos = poster.fetchMinerProvingDeadlineInfos(ctx, mids, ts)
		require.Len(t, dinfos, minerCount, "dlines for all mienrs")
		poster.handleHeadChange(ctx, mockTipSet(t, dl.Open-1), ts, dinfos)
		require.Equal(t, 0, countRunners(poster), "runner count")
	}
}

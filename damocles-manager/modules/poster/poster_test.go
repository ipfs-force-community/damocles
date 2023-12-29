package poster

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v12/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/testutil/testmodules"
)

var (
	invalidSender = modules.MustAddress(address.Undef)
)

var r = rand.New(rand.NewSource(time.Now().UnixNano()))

func randomDeadline() *dline.Info {
	periodStart := miner.WPoStProvingPeriod + abi.ChainEpoch(r.Intn(int(10*miner.WPoStProvingPeriod)))
	index := uint64(rand.Intn(int(miner.WPoStPeriodDeadlines)))

	return deadlineAt(periodStart, index)
}

func deadlineAt(periodStart abi.ChainEpoch, index uint64) *dline.Info {
	return dline.NewInfo(
		periodStart,
		index,
		0,
		miner.WPoStPeriodDeadlines,
		miner.WPoStProvingPeriod,
		miner.WPoStChallengeWindow,
		miner.WPoStChallengeLookback,
		miner.FaultDeclarationCutoff,
	)
}

func mockSafeConfig(count int) (*modules.SafeConfig, sync.Locker) {
	return testmodules.MockSafeConfig(count, func(mcfg *modules.MinerConfig) {
		mcfg.PoSt.Enabled = true
		fakerAddr := modules.GetFakeAddress()
		mcfg.PoSt.Sender = &fakerAddr
	})
}

const blkRaw = `{"Miner":"t038057","Ticket":{"VRFProof":"kfggWR2GcEbfTuJ20hkAFNRbF7xusDuAQR7XwTjJ2/gc1rwIDmaXbSVxXe4j1njcCBoMhmlYIn9D/BLqQuIOayMHPYvDmOJGc9M27Hwg1UZkiuJmXji+iM/JBNYaOA61"},"ElectionProof":{"WinCount":1,"VRFProof":"tI7cWWM9sGsKc69N9DjN41glaO5Hg7r742H56FPzg7szbhTrxj8kw0OsiJzcPJdiAa6D5jZ1S2WKoLK7lwg2R5zYvCRwwWLGDiExqbqsvqmH5z/e6YGpaD7ghTPRH1SR"},"BeaconEntries":[{"Round":2118576,"Data":"rintMKcvVAslYpn9DcshDBmlPN6hUR+wjvVQSkkVUK5klx1kOSpcDvzODSc2wXFQA7BVbEcXJW/5KLoL0KHx2alLUWDOwxhsIQnAydXdZqG8G76nTIgogthfIMgSGdB2"}],"WinPoStProof":[{"PoStProof":3,"ProofBytes":"t0ZgPHFv0ao9fVZJ/fxbBrzATmOiIv9/IueSyAjtcqEpxqWViqchaaxnz1afwzAbhahpfZsGiGWyc408WYh7Q8u0Aa52KGPmUNtf3pAvxWfsUDMz9QUfhLZVg/p8/PUVC/O/E7RBNq4YPrRK5b6Q8PVwzIOxGOS14ge6ys8Htq+LfNJbcqY676qOYF4lzMuMtQIe3CxMSAEaEBfNpHhAEs83dO6vll9MZKzcXYpNWeqmMIz4xSdF18StQq9vL/Lo"}],"Parents":[{"/":"bafy2bzacecf4wtqz3kgumeowhdulejk3xbfzgibfyhs42x4vx2guqgudem2hg"},{"/":"bafy2bzacebkpxh2k63xreigl6a3ggdr2adwk67b4zw5dddckhqex2tmha6hee"},{"/":"bafy2bzacecor3xq4ykmhhrgq55rdo5w7up65elc4qwx5uwjy25ffynidskbxw"},{"/":"bafy2bzacedr2mztmef65fodqzvyjcdnsgpcjthstseinll4maqg24avnv7ljo"}],"ParentWeight":"21779626255","Height":1164251,"ParentStateRoot":{"/":"bafy2bzacecypgutbewmyop2wfuafvxt7dm7ew4u3ssy2p4rn457f6ynrj2i6a"},"ParentMessageReceipts":{"/":"bafy2bzaceaflsspsxuxew2y4g6o72wp5i2ewp3fcolga6n2plw3gycam7s4lg"},"Messages":{"/":"bafy2bzaceanux5ivzlxzvhqxtwc5vkktcfqepubwtwgv26dowzbl3rtgqk54k"},"BLSAggregate":{"Type":2,"Data":"lQg9jBfYhY2vvjB/RPlWg6i+MBTlH1u0lmdasiab5BigsKAuZSeLNlTGbdoVZhAsDUT59ZdGsMmueHjafygDUN2KLhZoChFf6LQHH42PTSXFlkRVHvmKVz9DDU03FLMB"},"Timestamp":1658988330,"BlockSig":{"Type":2,"Data":"rMOv2tXKqV5VDOq5IQ35cP0cCAzGmaugVr/g5JTrilhAn4LYK0h6ByPL5cX5ONzlDTx9+zYZFteIzaenirZhw7G510Lh0J8lbTLP5X2EX251rEA8dpkPZPcNylzN0r8X"},"ForkSignaling":0,"ParentBaseFee":"100"}`

func mockTipSet(height abi.ChainEpoch) *types.TipSet {
	var blk types.BlockHeader
	err := json.Unmarshal([]byte(blkRaw), &blk)
	if err != nil {
		panic(err)
	}
	blk.Height = height

	ts, err := types.NewTipSet([]*types.BlockHeader{&blk})
	if err != nil {
		panic(err)
	}
	return ts
}

type mockSelecotor struct{}

func (mockSelecotor) Select(_ context.Context, mid abi.ActorID, senders []address.Address) (address.Address, error) {
	for _, sender := range senders {
		if sender != address.Undef {
			return sender, nil
		}
	}

	return address.Undef, fmt.Errorf("no valid senders for %d", mid)
}

func TestStartPostRunner(t *testing.T) {
	ctx := context.Background()

	dl := randomDeadline()
	dl.CurrentEpoch = abi.ChainEpoch(1000)
	dl = dl.NextNotElapsed()

	deps := postDeps{
		senderSelector: mockSelecotor{},
	}

	t.Run("run successful", func(t *testing.T) {
		mockExecutor := &mockExecutor{}
		NewPostExecutor = func(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info, cfg *modules.MinerPoStConfig) postExecutor {
			return mockExecutor
		}

		scfg, _ := mockSafeConfig(1)
		mid := scfg.Miners[0].Actor
		maddr, err := address.NewIDAddress(uint64(mid))
		require.NoError(t, err, "construct id address")
		subscriber, getRunner := StartWdPostScheduler(ctx, deps, mid, maddr, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, dl, scfg)

		subscriber(nil, mockTipSet(dl.Challenge))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence-1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {
			require.Equal(t, dl.Challenge+DefaultChallengeConfidence, ts.Height())
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, statePreparing, getRunner().State())
		require.Equal(t, 1, mockExecutor.handleFaultsCalled)

		mockPrepareResult := &PrepareResult{}
		mockExecutor.prepare = func() (*PrepareResult, error) {
			return mockPrepareResult, nil
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateProofGenerating, getRunner().State())
		require.Equal(t, 1, mockExecutor.prepareCalled)

		mockProof := make([]miner.SubmitWindowedPoStParams, 0, 1)
		mockProof = append(mockProof, miner.SubmitWindowedPoStParams{})
		mockExecutor.generatePoSt = func(r *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
			require.Equal(t, mockPrepareResult, r)
			return mockProof, nil
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForOpen, getRunner().State())
		require.Equal(t, 1, mockExecutor.generatePoStCalled)

		subscriber(nil, mockTipSet(dl.Open-1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForOpen, getRunner().State())

		subscriber(nil, mockTipSet(dl.Open+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateSubmitting, getRunner().State())

		mockExecutor.submitPoSts = func(ts *types.TipSet, proofs []miner.SubmitWindowedPoStParams) error {
			require.Equal(t, mockProof, proofs)
			return nil
		}
		subscriber(nil, mockTipSet(dl.Open+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateSubmitted, getRunner().State())
		require.Equal(t, 1, mockExecutor.submitPoStsCalled)

		subscriber(nil, mockTipSet(dl.Close-1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateSubmitted, getRunner().State())

		subscriber(nil, mockTipSet(dl.Close+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, StateEmpty, getRunner().State())

		subscriber(nil, mockTipSet(dl.Close+2))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {}
		subscriber(nil, mockTipSet(dl.Open+dl.WPoStProvingPeriod))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, 2, mockExecutor.handleFaultsCalled)
		require.Equal(t, statePreparing, getRunner().State())

	})

	t.Run("wdpost disable", func(t *testing.T) {
		scfg, _ := mockSafeConfig(1)
		scfg.Miners[0].PoSt.Enabled = false
		mid := scfg.Miners[0].Actor
		maddr, err := address.NewIDAddress(uint64(mid))
		require.NoError(t, err, "construct id address")

		subscriber, getRunner := StartWdPostScheduler(ctx, deps, mid, maddr, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, dl, scfg)

		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, nil, getRunner())

		subscriber(nil, mockTipSet(dl.Open+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, nil, getRunner())
	})

	t.Run("wdpost retry", func(t *testing.T) {
		// modify retry times
		DefaultRetryNum = 3

		mockExecutor := &mockExecutor{}
		NewPostExecutor = func(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info, cfg *modules.MinerPoStConfig) postExecutor {
			return mockExecutor
		}

		scfg, _ := mockSafeConfig(1)
		mid := scfg.Miners[0].Actor
		maddr, err := address.NewIDAddress(uint64(mid))
		require.NoError(t, err, "construct id address")
		subscriber, getRunner := StartWdPostScheduler(ctx, deps, mid, maddr, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, dl, scfg)

		subscriber(nil, mockTipSet(dl.Challenge-1))
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence-1))
		time.Sleep(100 * time.Millisecond)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {
			require.Equal(t, dl.Challenge+DefaultChallengeConfidence, ts.Height())
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, statePreparing, getRunner().State())
		require.Equal(t, 1, mockExecutor.handleFaultsCalled)

		mockPrepareResult := &PrepareResult{}
		mockExecutor.prepare = func() (*PrepareResult, error) {
			return nil, fmt.Errorf("first prepare error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, statePreparing, getRunner().State())
		require.Equal(t, 1, mockExecutor.prepareCalled)

		mockExecutor.prepare = func() (*PrepareResult, error) {
			return mockPrepareResult, fmt.Errorf("second prepare error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+2))
		require.Equal(t, statePreparing, getRunner().State())
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, 2, mockExecutor.prepareCalled)

		// recover before there times
		mockExecutor.prepare = func() (*PrepareResult, error) {
			return mockPrepareResult, nil
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+3))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateProofGenerating, getRunner().State())
		require.Equal(t, 3, mockExecutor.prepareCalled)

		mockExecutor.generatePoSt = func(r *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
			require.Equal(t, mockPrepareResult, r)
			return nil, fmt.Errorf("first generatePoSt error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateProofGenerating, getRunner().State())

		mockExecutor.generatePoSt = func(r *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
			return nil, fmt.Errorf("second generatePoSt error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+2))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateProofGenerating, getRunner().State())
		require.Equal(t, 2, mockExecutor.generatePoStCalled)

		mockExecutor.generatePoSt = func(r *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
			return nil, fmt.Errorf("third generatePoSt error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+3))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateProofGenerating, getRunner().State())
		require.Equal(t, 3, mockExecutor.generatePoStCalled)

		// exceed max retry times
		mockExecutor.generatePoSt = func(r *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
			return nil, fmt.Errorf("forth generatePoSt error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+4))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, StateAbort, getRunner().State())
		require.Equal(t, 4, mockExecutor.generatePoStCalled)

		subscriber(nil, mockTipSet(dl.Open+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, StateAbort, getRunner().State())
		require.Equal(t, 4, mockExecutor.generatePoStCalled)
	})

	t.Run("expired cancel", func(t *testing.T) {
		mockExecutor := &mockExecutor{}
		NewPostExecutor = func(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info, cfg *modules.MinerPoStConfig) postExecutor {
			return mockExecutor
		}

		scfg, _ := mockSafeConfig(1)
		mid := scfg.Miners[0].Actor
		maddr, err := address.NewIDAddress(uint64(mid))
		require.NoError(t, err, "construct id address")
		subscriber, getRunner := StartWdPostScheduler(ctx, deps, mid, maddr, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, dl, scfg)

		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {
			return
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, 1, mockExecutor.handleFaultsCalled)
		require.Equal(t, statePreparing, getRunner().State())

		mockExecutor.prepare = func() (*PrepareResult, error) {
			// simulate a long time task
			time.Sleep(1 * time.Hour)
			return nil, fmt.Errorf("first prepare error")
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+2))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, statePreparing, getRunner().State())

		for i := 0; i < 1025; i++ {
			subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+2))
		}

		subscriber(nil, mockTipSet(dl.Close+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, StateEmpty, getRunner().State())

		subscriber(nil, mockTipSet(dl.Close+2))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		subscriber(nil, mockTipSet(dl.Challenge+dl.WPoStProvingPeriod))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {
			return
		}
		subscriber(nil, mockTipSet(dl.Challenge+dl.WPoStProvingPeriod+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, 2, mockExecutor.handleFaultsCalled)
		require.Equal(t, statePreparing, getRunner().State())
	})

	t.Run("revert", func(t *testing.T) {
		mockExecutor := &mockExecutor{}
		NewPostExecutor = func(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info, cfg *modules.MinerPoStConfig) postExecutor {
			return mockExecutor
		}

		scfg, _ := mockSafeConfig(1)
		mid := scfg.Miners[0].Actor
		maddr, err := address.NewIDAddress(uint64(mid))
		require.NoError(t, err, "construct id address")
		subscriber, getRunner := StartWdPostScheduler(ctx, deps, mid, maddr, abi.RegisteredPoStProof_StackedDrgWindow2KiBV1, dl, scfg)

		subscriber(nil, mockTipSet(dl.Challenge))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {
			return
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, statePreparing, getRunner().State())

		subscriber(mockTipSet(dl.Challenge-1), mockTipSet(dl.Challenge+DefaultChallengeConfidence+2))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, StateEmpty, getRunner().State())

		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, stateWaitForChallenge, getRunner().State())

		mockExecutor.handleFaults = func(ts *types.TipSet) {
			return
		}
		subscriber(nil, mockTipSet(dl.Challenge+DefaultChallengeConfidence+1))
		time.Sleep(time.Millisecond * 100)
		require.Equal(t, 2, mockExecutor.handleFaultsCalled)
		require.Equal(t, statePreparing, getRunner().State())
	})

}

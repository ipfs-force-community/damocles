package poster

import (
	"context"
	"sync/atomic"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
)

func mockRunnerConstructor(runner *mockRunner) runnerConstructor {
	return func(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info) PoStRunner {
		return runner
	}
}

type mockRunner struct {
	started  uint32
	submited uint32
	aborted  uint32
}

func (m *mockRunner) start(pcfg *modules.MinerPoStConfig, ts *types.TipSet) {
	atomic.AddUint32(&m.started, 1)
}

func (m *mockRunner) submit(pcfg *modules.MinerPoStConfig, ts *types.TipSet) {
	atomic.AddUint32(&m.submited, 1)
}

func (m *mockRunner) abort() { atomic.AddUint32(&m.aborted, 1) }

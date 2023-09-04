package mock

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
)

var _ core.MinerAPI = (*minerAPI)(nil)

func NewMinerAPI(miner abi.ActorID, proofType abi.RegisteredSealProof) core.MinerAPI {
	return &minerAPI{
		minerID:   miner,
		proofType: proofType,
	}
}

type minerAPI struct {
	minerID   abi.ActorID
	proofType abi.RegisteredSealProof
}

func (m *minerAPI) GetInfo(_ context.Context, minerID abi.ActorID) (*core.MinerInfo, error) {
	if minerID != m.minerID {
		return nil, fmt.Errorf("miner id '%s' not found", minerID)
	}

	ppt, _ := m.proofType.RegisteredWindowPoStProof()
	ss, _ := m.proofType.ProofSize()

	return &core.MinerInfo{
		ID:                  m.minerID,
		Addr:                address.Address{},
		SectorSize:          abi.SectorSize(ss),
		WindowPoStProofType: ppt,
		SealProofType:       m.proofType,
	}, nil
}

func (m *minerAPI) GetMinerConfig(_ context.Context, minerID abi.ActorID) (*modules.MinerConfig, error) {
	if minerID != m.minerID {
		return nil, fmt.Errorf("miner id '%s' not found", minerID)
	}
	cfg := modules.DefaultMinerConfig(true)
	return &cfg, nil
}

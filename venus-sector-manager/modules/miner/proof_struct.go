package miner

import (
	"context"

	"github.com/ipfs-force-community/venus-gateway/proofevent"
	"github.com/ipfs-force-community/venus-gateway/types"
)

type ProofEventStruct struct {
}

func (ProofEventStruct) ResponseProofEvent(ctx context.Context, resp *types.ResponseEvent) error {
	return nil
}

func (ProofEventStruct) ListenProofEvent(ctx context.Context, policy *proofevent.ProofRegisterPolicy) (<-chan *types.RequestEvent, error) {
	return nil, nil
}

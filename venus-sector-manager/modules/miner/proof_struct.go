package miner

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/ipfs-force-community/venus-gateway/proofevent"
	"github.com/ipfs-force-community/venus-gateway/types"
)

var ErrNotSupported = xerrors.New("method not supported")

type ProofEventStruct struct {
	Internal struct {
		ResponseProofEvent func(ctx context.Context, resp *types.ResponseEvent) error
		ListenProofEvent   func(ctx context.Context, policy *proofevent.ProofRegisterPolicy) (<-chan *types.RequestEvent, error)
	}
}

func (pe *ProofEventStruct) ResponseProofEvent(ctx context.Context, resp *types.ResponseEvent) error {
	if pe.Internal.ResponseProofEvent == nil {
		return ErrNotSupported
	}
	return pe.Internal.ResponseProofEvent(ctx, resp)
}

func (pe *ProofEventStruct) ListenProofEvent(ctx context.Context, policy *proofevent.ProofRegisterPolicy) (<-chan *types.RequestEvent, error) {
	if pe.Internal.ListenProofEvent == nil {
		return nil, ErrNotSupported
	}
	return pe.Internal.ListenProofEvent(ctx, policy)
}


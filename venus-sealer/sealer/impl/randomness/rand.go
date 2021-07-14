package randomness

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var _ api.RandomnessAPI = (*Randomness)(nil)

func New(capi chain.API) (api.RandomnessAPI, error) {
	return &Randomness{
		api: capi,
	}, nil
}

type Randomness struct {
	api chain.API
}

func (r *Randomness) GetTicket(ctx context.Context, tsk types.TipSetKey, epoch abi.ChainEpoch, entropy []byte) (api.Ticket, error) {
	if tsk == types.EmptyTSK {
		ts, err := r.api.ChainHead(ctx)
		if err != nil {
			return api.Ticket{}, err
		}

		tsk = ts.Key()
	}

	rand, err := r.api.ChainGetRandomnessFromTickets(ctx, tsk, crypto.DomainSeparationTag_SealRandomness, epoch, entropy)
	if err != nil {
		return api.Ticket{}, err
	}

	return api.Ticket{
		Ticket: rand,
		Epoch:  epoch,
	}, nil
}

func (r *Randomness) GetSeed(ctx context.Context, tsk types.TipSetKey, epoch abi.ChainEpoch, entropy []byte) (api.Seed, error) {
	if tsk == types.EmptyTSK {
		ts, err := r.api.ChainHead(ctx)
		if err != nil {
			return api.Seed{}, err
		}

		tsk = ts.Key()
	}

	rand, err := r.api.ChainGetRandomnessFromBeacon(ctx, tsk, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, epoch, entropy)
	if err != nil {
		return api.Seed{}, err
	}

	return api.Seed{
		Seed:  rand,
		Epoch: epoch,
	}, nil
}

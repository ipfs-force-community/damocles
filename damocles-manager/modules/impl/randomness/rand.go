package randomness

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
)

var _ core.RandomnessAPI = (*Randomness)(nil)

func New(capi chain.API) (core.RandomnessAPI, error) {
	return &Randomness{
		api: capi,
	}, nil
}

type Randomness struct {
	api chain.API
}

func (*Randomness) getRandomnessEntropy(mid abi.ActorID) ([]byte, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := maddr.MarshalCBOR(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (r *Randomness) GetTicket(
	ctx context.Context,
	tsk types.TipSetKey,
	epoch abi.ChainEpoch,
	mid abi.ActorID,
) (core.Ticket, error) {
	entropy, err := r.getRandomnessEntropy(mid)
	if err != nil {
		return core.Ticket{}, err
	}

	if tsk == types.EmptyTSK {
		ts, err := r.api.ChainHead(ctx)
		if err != nil {
			return core.Ticket{}, err
		}

		tsk = ts.Key()
	}

	rand, err := r.api.StateGetRandomnessFromTickets(
		ctx,
		crypto.DomainSeparationTag_SealRandomness,
		epoch,
		entropy,
		tsk,
	)
	if err != nil {
		return core.Ticket{}, err
	}

	return core.Ticket{
		Ticket: rand,
		Epoch:  epoch,
	}, nil
}

func (r *Randomness) GetSeed(
	ctx context.Context,
	tsk types.TipSetKey,
	epoch abi.ChainEpoch,
	mid abi.ActorID,
) (core.Seed, error) {
	entropy, err := r.getRandomnessEntropy(mid)
	if err != nil {
		return core.Seed{}, err
	}

	if tsk == types.EmptyTSK {
		ts, err := r.api.ChainHead(ctx)
		if err != nil {
			return core.Seed{}, err
		}

		tsk = ts.Key()
	}

	rand, err := r.api.StateGetRandomnessFromBeacon(
		ctx,
		crypto.DomainSeparationTag_InteractiveSealChallengeSeed,
		epoch,
		entropy,
		tsk,
	)
	if err != nil {
		return core.Seed{}, err
	}

	return core.Seed{
		Seed:  rand,
		Epoch: epoch,
	}, nil
}

func (r *Randomness) GetWindowPoStChanlleengeRand(
	ctx context.Context,
	tsk types.TipSetKey,
	epoch abi.ChainEpoch,
	mid abi.ActorID,
) (core.WindowPoStRandomness, error) {
	entropy, err := r.getRandomnessEntropy(mid)
	if err != nil {
		return core.WindowPoStRandomness{}, err
	}

	if tsk == types.EmptyTSK {
		ts, err := r.api.ChainHead(ctx)
		if err != nil {
			return core.WindowPoStRandomness{}, err
		}

		tsk = ts.Key()
	}

	rand, err := r.api.StateGetRandomnessFromBeacon(
		ctx,
		crypto.DomainSeparationTag_WindowedPoStChallengeSeed,
		epoch,
		entropy,
		tsk,
	)
	if err != nil {
		return core.WindowPoStRandomness{}, err
	}

	return core.WindowPoStRandomness{
		Rand:  rand,
		Epoch: epoch,
	}, nil
}

func (r *Randomness) GetWindowPoStCommitRand(
	ctx context.Context,
	tsk types.TipSetKey,
	epoch abi.ChainEpoch,
) (core.WindowPoStRandomness, error) {
	if tsk == types.EmptyTSK {
		ts, err := r.api.ChainHead(ctx)
		if err != nil {
			return core.WindowPoStRandomness{}, err
		}

		tsk = ts.Key()
	}

	rand, err := r.api.StateGetRandomnessFromTickets(ctx, crypto.DomainSeparationTag_PoStChainCommit, epoch, nil, tsk)
	if err != nil {
		return core.WindowPoStRandomness{}, err
	}

	return core.WindowPoStRandomness{
		Rand:  rand,
		Epoch: epoch,
	}, nil
}

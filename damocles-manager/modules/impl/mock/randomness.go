package mock

import (
	"context"
	"crypto/rand"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
)

var _ core.RandomnessAPI = (*random)(nil)

func NewRandomness() core.RandomnessAPI {
	var ticket, seed, wdchallenge, wdcommit [32]byte
	_, _ = rand.Read(ticket[:])
	_, _ = rand.Read(seed[:])
	_, _ = rand.Read(wdchallenge[:])
	_, _ = rand.Read(wdcommit[:])

	return &random{
		ticket:      ticket,
		seed:        seed,
		wdchallenge: wdchallenge,
		wdcommit:    wdcommit,
	}
}

type random struct {
	ticket      [32]byte
	seed        [32]byte
	wdchallenge [32]byte
	wdcommit    [32]byte
}

func (r *random) GetTicket(
	_ context.Context,
	_ types.TipSetKey,
	epoch abi.ChainEpoch,
	_ abi.ActorID,
) (core.Ticket, error) {
	return core.Ticket{
		Ticket: r.ticket[:],
		Epoch:  epoch,
	}, nil
}

func (r *random) GetSeed(_ context.Context, _ types.TipSetKey, epoch abi.ChainEpoch, _ abi.ActorID) (core.Seed, error) {
	return core.Seed{
		Seed:  r.seed[:],
		Epoch: epoch,
	}, nil
}

func (r *random) GetWindowPoStChanlleengeRand(
	_ context.Context,
	_ types.TipSetKey,
	epoch abi.ChainEpoch,
	_ abi.ActorID,
) (core.WindowPoStRandomness, error) {
	return core.WindowPoStRandomness{
		Rand:  r.wdchallenge[:],
		Epoch: epoch,
	}, nil
}

func (r *random) GetWindowPoStCommitRand(
	_ context.Context,
	_ types.TipSetKey,
	epoch abi.ChainEpoch,
) (core.WindowPoStRandomness, error) {
	return core.WindowPoStRandomness{
		Rand:  r.wdcommit[:],
		Epoch: epoch,
	}, nil
}

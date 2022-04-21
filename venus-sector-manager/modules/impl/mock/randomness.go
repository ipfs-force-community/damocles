package mock

import (
	"context"
	"crypto/rand"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
)

var _ api.RandomnessAPI = (*random)(nil)

func NewRandomness() api.RandomnessAPI {
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

func (r *random) GetTicket(ctx context.Context, tsk types.TipSetKey, epoch abi.ChainEpoch, mid abi.ActorID) (api.Ticket, error) {
	return api.Ticket{
		Ticket: r.ticket[:],
		Epoch:  epoch,
	}, nil
}

func (r *random) GetSeed(ctx context.Context, tsk types.TipSetKey, epoch abi.ChainEpoch, mid abi.ActorID) (api.Seed, error) {
	return api.Seed{
		Seed:  r.seed[:],
		Epoch: epoch,
	}, nil
}

func (r *random) GetWindowPoStChanlleengeRand(ctx context.Context, tsk types.TipSetKey, epoch abi.ChainEpoch, mid abi.ActorID) (api.WindowPoStRandomness, error) {
	return api.WindowPoStRandomness{
		Rand:  r.wdchallenge[:],
		Epoch: epoch,
	}, nil
}

func (r *random) GetWindowPoStCommitRand(ctx context.Context, tsk types.TipSetKey, epoch abi.ChainEpoch) (api.WindowPoStRandomness, error) {
	return api.WindowPoStRandomness{
		Rand:  r.wdcommit[:],
		Epoch: epoch,
	}, nil
}

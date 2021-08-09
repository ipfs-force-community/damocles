package mock

import (
	"context"
	"math/rand"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var _ api.RandomnessAPI = (*random)(nil)

func NewRandomness() api.RandomnessAPI {
	var ticket, seed [32]byte
	rand.Read(ticket[:])
	rand.Read(seed[:])
	return &random{
		ticket: ticket,
		seed:   seed,
	}
}

type random struct {
	ticket [32]byte
	seed   [32]byte
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

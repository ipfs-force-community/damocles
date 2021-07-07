package mock

import (
	"context"
	"math/rand"
	"sync/atomic"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

var _ api.RandomnessAPI = (*random)(nil)

func NewRandomness() api.RandomnessAPI {
	return &random{}
}

type random struct {
	ticketEpoch int64
	seedEpoch   int64
}

func (r *random) GetTicket(context.Context) (api.Ticket, error) {
	ticket := api.Ticket{
		Epoch:  abi.ChainEpoch(atomic.AddInt64(&r.ticketEpoch, 1)),
		Ticket: make(abi.Randomness, 32),
	}

	rand.Read(ticket.Ticket[:])
	return ticket, nil
}

func (r *random) GetSeed(context.Context) (api.Seed, error) {
	seed := api.Seed{
		Epoch: abi.ChainEpoch(atomic.AddInt64(&r.seedEpoch, 1)),
		Seed:  make(abi.Randomness, 32),
	}

	rand.Read(seed.Seed[:])
	return seed, nil
}

package api

import (
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
)

type Sector struct {
	SectorID   abi.SectorID
	SectorType abi.RegisteredSealProof

	// precommit info
	TicketValue abi.SealRandomness
	TicketEpoch abi.ChainEpoch

	CommR *cid.Cid
	CommD *cid.Cid

	Deals        Deals
	PreCommitCid *cid.Cid

	// commit info

	// WaitSeed
	SeedValue abi.InteractiveSealRandomness
	SeedEpoch abi.ChainEpoch

	Proof     []byte
	CommitCid *cid.Cid

	// when restart，it should be add to send loop
	// this flag is set to true when add to send and clean when send a msg
	NeedSend bool

	// debug
	Logs []Log // everytime change this struct, should log!
}

func (s Sector) DealIDs() []abi.DealID {
	res := make([]abi.DealID, len(s.Deals))
	for i := range s.Deals {
		res[i] = s.Deals[i].ID
	}
	return res
}

type Log struct {
	Timestamp uint64
	Trace     string // for errors

	Message string

	// additional data (Event info)
	Kind string
}

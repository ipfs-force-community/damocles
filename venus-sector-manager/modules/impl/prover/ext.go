package prover

import (
	"encoding/json"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
)

const (
	ExtProcessorNameWindostPoSt = "wdpost"
)

type ExtRequest struct {
	ID   uint64          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type ExtResponse struct {
	ID     uint64          `json:"id"`
	ErrMsg *string         `json:"err_msg"`
	Result json.RawMessage `json:"result"`
}

func (r *ExtResponse) SetResult(res interface{}) {
	b, err := json.Marshal(res)
	if err != nil {
		errMsg := err.Error()
		r.ErrMsg = &errMsg
		return
	}

	r.Result = b
}

type WindowPoStData struct {
	Miner      abi.ActorID
	Sectors    SortedPrivateSectorInfo
	Randomness abi.PoStRandomness
}

type WindowPoStResult struct {
	Proof   []builtin.PoStProof
	Skipped []abi.SectorID
}

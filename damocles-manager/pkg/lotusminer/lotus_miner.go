package lotusminer

import (
	"context"
	"fmt"

	jsonrpc "github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	sealing "github.com/filecoin-project/lotus/storage/pipeline"
	vapi "github.com/filecoin-project/venus/venus-shared/api"
)

const (
	FinalizeSector        = sealing.FinalizeSector
	Proving               = sealing.Proving
	FinalizeReplicaUpdate = sealing.FinalizeReplicaUpdate
	UpdateActivating      = sealing.UpdateActivating
	ReleaseSectorKey      = sealing.ReleaseSectorKey
	SectorStorePrefix     = sealing.SectorStorePrefix
)

var (
	StorageCounterDSPrefix = sealing.StorageCounterDSPrefix
	SafePiece              = sealing.SafePiece
)

type (
	StorageMiner      = api.StorageMiner
	SectorInfo        = api.SectorInfo
	SectorPiece       = api.SectorPiece
	SectorSealingInfo = sealing.SectorInfo
	SectorState       = sealing.SectorState
	PieceDealInfo     = api.PieceDealInfo
	SafeSectorPiece   = sealing.SafeSectorPiece
)

func New(ctx context.Context, addr string, token string) (StorageMiner, jsonrpc.ClientCloser, error) {
	ainfo := vapi.NewAPIInfo(addr, token)

	var a api.StorageMinerStruct
	apiAddr, err := ainfo.DialArgs(vapi.VerString(0))
	if err != nil {
		return nil, nil, fmt.Errorf("get api addr: %w", err)
	}

	closer, err := jsonrpc.NewMergeClient(ctx, apiAddr, "Filecoin", vapi.GetInternalStructs(&a), ainfo.AuthHeader(), jsonrpc.WithRetry(true))
	if err != nil {
		return nil, nil, fmt.Errorf("dial: %w", err)
	}

	return &a, closer, nil
}

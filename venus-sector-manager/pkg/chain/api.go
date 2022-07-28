package chain

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"

	v1 "github.com/filecoin-project/venus/venus-shared/api/chain/v1"
	"github.com/filecoin-project/venus/venus-shared/types"
)

const (
	HCRevert  = types.HCRevert
	HCApply   = types.HCApply
	HCCurrent = types.HCCurrent
)

type (
	HeadChange = types.HeadChange
	Partition  = types.Partition
)

type API = v1.FullNode

func New(ctx context.Context, api, token string) (API, jsonrpc.ClientCloser, error) {
	client, closer, err := v1.DialFullNodeRPC(ctx, api, token, nil, jsonrpc.WithRetry(true))
	if err != nil {
		return nil, nil, err
	}

	return client, closer, nil
}

type MockStruct = v1.FullNodeStruct

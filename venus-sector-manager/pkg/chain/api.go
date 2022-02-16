package chain

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	"github.com/filecoin-project/venus/venus-shared/api/chain/v1"
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

type API struct {
	v1.IChainInfoStruct
	v1.ISyncerStruct
	v1.IActorStruct
	v1.IBlockStoreStruct
	v1.IMinerStateStruct
}

func New(ctx context.Context, api, token string) (API, jsonrpc.ClientCloser, error) {
	ainfo := apiinfo.NewAPIInfo(api, token)
	addr, err := ainfo.DialArgs("v1")
	if err != nil {
		return API{}, nil, err
	}

	var node API
	closer, err := jsonrpc.NewClient(ctx, addr, "Filecoin", &node, ainfo.AuthHeader())
	if err != nil {
		return API{}, nil, err
	}

	return node, closer, nil
}

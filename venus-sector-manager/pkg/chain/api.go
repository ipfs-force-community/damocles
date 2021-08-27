package chain

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/venus/app/client"
	"github.com/filecoin-project/venus/app/submodule/apitypes"
	"github.com/filecoin-project/venus/pkg/chain"
	"github.com/ipfs-force-community/venus-common-utils/apiinfo"
)

const (
	HCRevert  = chain.HCRevert
	HCApply   = chain.HCApply
	HCCurrent = chain.HCCurrent
)

type (
	HeadChange = chain.HeadChange
	Partition  = apitypes.Partition
)

type API struct {
	client.IChainInfoStruct
	client.ISyncerStruct
	client.IActorStruct
	client.IBlockStoreStruct
	client.IMinerStateStruct
}

func New(ctx context.Context, api, token string) (API, jsonrpc.ClientCloser, error) {
	ainfo := apiinfo.NewAPIInfo(api, token)
	addr, err := ainfo.DialArgs("v0")
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

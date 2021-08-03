package chain

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/venus/app/client"
	// apifacev0 "github.com/filecoin-project/venus/app/submodule/apiface/v0api"
	"github.com/ipfs-force-community/venus-common-utils/apiinfo"
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

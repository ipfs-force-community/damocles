package messager

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/venus-messager/api/client"
	"github.com/filecoin-project/venus-messager/types"
	"github.com/ipfs-force-community/venus-common-utils/apiinfo"
)

type MsgMeta = types.MsgMeta

var (
	OnChainMsg = types.OnChainMsg
	FailedMsg  = types.FailedMsg
)

type API interface {
	client.IMessager
}

func New(ctx context.Context, api, token string) (API, jsonrpc.ClientCloser, error) {
	ainfo := apiinfo.NewAPIInfo(api, token)
	addr, err := ainfo.DialArgs("v0")
	if err != nil {
		return nil, nil, err
	}

	return client.NewMessageRPC(ctx, addr, ainfo.AuthHeader())
}

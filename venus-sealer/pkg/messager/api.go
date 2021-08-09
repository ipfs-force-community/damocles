package messager

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/venus-messager/api/client"
	"github.com/filecoin-project/venus-messager/types"
	vtypes "github.com/filecoin-project/venus/pkg/types"
	"github.com/ipfs-force-community/venus-common-utils/apiinfo"
)

type (
	UnsignedMessage = vtypes.UnsignedMessage
	MsgMeta         = types.MsgMeta
	Message         = types.Message
	MessageReceipt  = vtypes.MessageReceipt
)

var MsgStateToString = types.MsgStateToString

var MessageState = struct {
	UnKnown,
	UnFillMsg,
	FillMsg,
	OnChainMsg,
	FailedMsg,
	ReplacedMsg,
	NoWalletMsg types.MessageState
}{
	types.UnKnown,
	types.UnFillMsg,
	types.FillMsg,
	types.OnChainMsg,
	types.FailedMsg,
	types.ReplacedMsg,
	types.NoWalletMsg,
}

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

package messager

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	"github.com/filecoin-project/venus-messager/api/client"

	"github.com/filecoin-project/venus/venus-shared/types"
	mtypes "github.com/filecoin-project/venus/venus-shared/types/messager"
)

type (
	UnsignedMessage = types.Message
	MsgMeta         = mtypes.SendSpec
	Message         = mtypes.Message
	MessageReceipt  = types.MessageReceipt
)

var MessageStateToString = mtypes.MessageStateToString

var MessageState = struct {
	UnKnown,
	UnFillMsg,
	FillMsg,
	OnChainMsg,
	FailedMsg,
	ReplacedMsg,
	NoWalletMsg mtypes.MessageState
}{
	mtypes.UnKnown,
	mtypes.UnFillMsg,
	mtypes.FillMsg,
	mtypes.OnChainMsg,
	mtypes.FailedMsg,
	mtypes.ReplacedMsg,
	mtypes.NoWalletMsg,
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

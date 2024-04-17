package messager

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"

	mapi "github.com/filecoin-project/venus/venus-shared/api/messager"
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
	//revive:disable-next-line:line-length-limit
	NonceConflictMsg mtypes.MessageState // Has been on-chain after being replaced by off-chain services, usually by `mpool replace`, eg. `venus mpool replace`
}{
	mtypes.UnKnown,
	mtypes.UnFillMsg,
	mtypes.FillMsg,
	mtypes.OnChainMsg,
	mtypes.FailedMsg,
	mtypes.NonceConflictMsg,
}

type API = mapi.IMessager

func New(ctx context.Context, api, token string) (API, jsonrpc.ClientCloser, error) {
	return mapi.DialIMessagerRPC(ctx, api, token, nil, jsonrpc.WithRetry(true))
}

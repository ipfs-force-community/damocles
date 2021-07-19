package message_client

import (
	"context"

	"github.com/filecoin-project/venus-messager/api/client"
	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
)

func NewMessageClient(ctx context.Context, cfg *sealer.Config, locker confmgr.RLocker) (client.IMessager, error) {
	locker.Lock()
	msgClientCfg := cfg.MessagerClient
	locker.Lock()

	apiInfo := apiinfo.NewAPIInfo(msgClientCfg.Api, msgClientCfg.Token)
	addr, err := apiInfo.DialArgs("v0")
	if err != nil {
		return nil, err
	}

	client, _, err := client.NewMessageRPC(ctx, addr, apiInfo.AuthHeader())
	return client, err
}

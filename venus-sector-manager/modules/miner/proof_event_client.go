package miner

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	"github.com/ipfs-force-community/venus-gateway/proofevent"
)

func NewProofEventClient(lc fx.Lifecycle, url, token string) (proofevent.IProofEventAPI, error) {
	pvc := &ProofEventStruct{}
	apiInfo := apiinfo.APIInfo{
		Addr:  url,
		Token: []byte(token),
	}

	addr, err := apiInfo.DialArgs("v0")
	if err != nil {
		return nil, err
	}

	closer, err := jsonrpc.NewMergeClient(context.Background(), addr, "Gateway", []interface{}{pvc}, apiInfo.AuthHeader())
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			closer()
			return nil
		},
	})
	return pvc, nil
}

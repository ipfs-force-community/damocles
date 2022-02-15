package miner

import (
	"context"

	"go.uber.org/fx"

	"github.com/filecoin-project/go-jsonrpc"

	"github.com/ipfs-force-community/venus-gateway/proofevent"

	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	"github.com/filecoin-project/venus/venus-shared/api"
)

func NewProofEventClient(lc fx.Lifecycle, url, token string) (proofevent.IProofEventAPI, error) {
	apiInfo := apiinfo.APIInfo{
		Addr:  url,
		Token: []byte(token),
	}

	addr, err := apiInfo.DialArgs("v0")
	if err != nil {
		return nil, err
	}

	var pvc ProofEventStruct
	closer, err := jsonrpc.NewMergeClient(context.Background(), addr, "Gateway", api.GetInternalStructs(&pvc), apiInfo.AuthHeader())
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(_ context.Context) error {
			closer()
			return nil
		},
	})
	return &pvc, nil
}

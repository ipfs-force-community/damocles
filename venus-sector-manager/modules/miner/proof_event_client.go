package miner

import (
	"context"

	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	"github.com/filecoin-project/venus/venus-shared/api/gateway"
)

func NewProofEventClient(lc fx.Lifecycle, url, token string) (gateway.IProofEventAPI, error) {
	apiInfo := apiinfo.APIInfo{
		Addr:  url,
		Token: []byte(token),
	}

	addr, err := apiInfo.DialArgs("v0")
	if err != nil {
		return nil, err
	}

	pvc, closer, err := gateway.NewIProofEventAPIRPC(context.Background(), addr, apiInfo.AuthHeader())
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

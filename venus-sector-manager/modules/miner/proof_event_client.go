package miner

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-common-utils/apiinfo"

	gateway "github.com/filecoin-project/venus/venus-shared/api/gateway/v1"
)

func NewProofEventClient(lc fx.Lifecycle, url, token string) (gateway.IGateway, error) {
	apiInfo := apiinfo.APIInfo{
		Addr:  url,
		Token: []byte(token),
	}

	addr, err := apiInfo.DialArgs(fmt.Sprintf("v%d", gateway.MajorVersion))
	if err != nil {
		return nil, err
	}

	pvc, closer, err := gateway.NewIGatewayRPC(context.Background(), addr, apiInfo.AuthHeader())
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

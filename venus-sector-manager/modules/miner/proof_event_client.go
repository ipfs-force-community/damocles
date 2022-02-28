package miner

import (
	"context"

	"go.uber.org/fx"

	gateway "github.com/filecoin-project/venus/venus-shared/api/gateway/v1"
)

func NewProofEventClient(lc fx.Lifecycle, url, token string) (gateway.IGateway, error) {
	pvc, closer, err := gateway.DialIGatewayRPC(context.Background(), url, token, nil)
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

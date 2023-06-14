package miner

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc"

	"go.uber.org/fx"

	gateway "github.com/filecoin-project/venus/venus-shared/api/gateway/v2"
)

func NewProofEventClient(lc fx.Lifecycle, url, token string) (gateway.IGateway, error) {
	pvc, closer, err := gateway.DialIGatewayRPC(context.Background(), url, token, nil, jsonrpc.WithRetry(true))
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

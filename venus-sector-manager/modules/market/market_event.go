package market

import (
	"context"
	"encoding/json"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/venus/venus-shared/api/gateway/v2"
	"github.com/filecoin-project/venus/venus-shared/types"

	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"

	"go.uber.org/fx"
)

var log = logging.New("wallet_event")

type IMarketEvent interface {
	// OnUnseal register a hook function which will be triggered when a unseal request is received
	OnUnseal(f func(ctx context.Context, eventId types.UUID, req *UnsealRequest))
}

//go:generate mockgen -destination=./market_event_mock.go -package=market github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market IMarketEvent

var _ IMarketEvent = &MarketEvent{}

type UnsealRequest = gtypes.UnsealRequest

// MarketEvent handle event which come from gateway but originated from market
// It will trigger up the hook function registered in when the event is received
type MarketEvent struct {
	clients map[string]*MarketEventClient
	eventCh chan *gtypes.RequestEvent

	onUnseal []func(ctx context.Context, eventId types.UUID, req *UnsealRequest)
}

func NewMarketEvent(ctx context.Context, lc fx.Lifecycle, urls []string, token string, miners []address.Address) (*MarketEvent, error) {
	clients := make(map[string]*MarketEventClient, len(urls))
	eventCh := make(chan *gtypes.RequestEvent, 1)

	// init gateway client and listen on miners
	for _, url := range urls {
		api, closer, err := gateway.DialIGatewayRPC(ctx, url, token, nil, jsonrpc.WithRetry(true))
		if err != nil {
			return nil, err
		}
		lc.Append(fx.Hook{
			OnStop: func(_ context.Context) error {
				closer()
				return nil
			},
		})

		client := &MarketEventClient{
			IMarketServiceProvider: api,
			url:                    url,
		}

		for _, miner := range miners {
			client.ListenOnMiner(ctx, miner, eventCh)
		}
		clients[url] = client
	}

	ret := &MarketEvent{
		clients: clients,
		eventCh: eventCh,
	}

	go ret.handleEvent(ctx)

	return ret, nil
}

func (m *MarketEvent) handleEvent(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-m.eventCh:
			switch e.Method {
			case "SectorsUnsealPiece":
				req := UnsealRequest{}
				err := json.Unmarshal(e.Payload, &req)
				if err != nil {
					m.ResponseMarketEvent(ctx, &gtypes.ResponseEvent{
						ID:      e.ID,
						Payload: nil,
						Error:   err.Error(),
					})
					continue
				}
				for _, f := range m.onUnseal {
					f(ctx, e.ID, &req)
				}
			}
		}
	}
}

func (m *MarketEvent) OnUnseal(f func(ctx context.Context, eventId types.UUID, req *UnsealRequest)) {
	m.onUnseal = append(m.onUnseal, f)
}

func (m *MarketEvent) ResponseMarketEvent(ctx context.Context, resp *gtypes.ResponseEvent) error {
	return nil
}

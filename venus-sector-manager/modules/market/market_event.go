package market

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-jsonrpc"

	"github.com/filecoin-project/venus/venus-shared/api/gateway/v2"
	"github.com/filecoin-project/venus/venus-shared/types"

	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("market_event")

type IMarketEvent interface {
	// OnUnseal register a hook function which will be triggered when a unseal request is received
	OnUnseal(f func(ctx context.Context, eventID types.UUID, req *UnsealRequest))
	RespondUnseal(ctx context.Context, eventID types.UUID, err error) error
}

//go:generate mockgen -destination=./market_event_mock.go -package=market github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market IMarketEvent

var _ IMarketEvent = &MarketEvent{}

type UnsealRequest = gtypes.UnsealRequest
type GatewayEvent struct {
	URL string
	gtypes.RequestEvent
}

// MarketEvent handle event which come from gateway but originated from market
// It will trigger up the hook function registered in when the event is received
type MarketEvent struct {
	clients map[string]*EventClient
	eventCh chan *GatewayEvent

	recorder *EventRecorder

	onUnseal []func(ctx context.Context, eventId types.UUID, req *UnsealRequest)
}

func NewMarketEvent(ctx context.Context, urls []string, token string, miners []address.Address, kv kvstore.KVStore) (*MarketEvent, func(), error) {
	clients := make(map[string]*EventClient, len(urls))
	eventCh := make(chan *GatewayEvent, 1)
	closers := make([]func(), 0, len(urls))

	// init gateway client and listen on miners
	for _, url := range urls {
		api, closer, err := gateway.DialIGatewayRPC(ctx, url, token, nil, jsonrpc.WithRetry(true))
		if err != nil {
			return nil, nil, err
		}
		closers = append(closers, closer)

		client := &EventClient{
			IMarketServiceProvider: api,
			url:                    url,
		}

		for _, miner := range miners {
			client.ListenOnMiner(ctx, miner, eventCh)
		}
		clients[url] = client
	}

	recorder := &EventRecorder{
		kv:    kv,
		kvMux: sync.Mutex{},
	}

	marketEvent := &MarketEvent{
		clients:  clients,
		eventCh:  eventCh,
		recorder: recorder,
	}

	go marketEvent.handleEvent(ctx)

	closer := func() {
		for _, c := range closers {
			c()
		}
	}

	return marketEvent, closer, nil
}

func (m *MarketEvent) handleEvent(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-m.eventCh:
			log.Infof("received event from %s", e.URL)
			// record event
			err := m.recorder.Add(ctx, e.ID, e.URL)
			if err != nil {
				log.Errorf("record event %s failed: %s", e.ID, err)
				continue
			}

			switch e.Method {
			case "SectorsUnsealPiece":
				req := UnsealRequest{}
				err := json.Unmarshal(e.Payload, &req)
				if err != nil {
					rErr := m.ResponseMarketEvent(ctx, &gtypes.ResponseEvent{
						ID:      e.ID,
						Payload: nil,
						Error:   err.Error(),
					})
					log.Errorf("response event %s failed: %s", e.ID, rErr)
					continue
				}
				for _, hook := range m.onUnseal {
					hook(ctx, e.ID, &req)
				}
			}
		}
	}
}

func (m *MarketEvent) OnUnseal(f func(ctx context.Context, eventId types.UUID, req *UnsealRequest)) {
	m.onUnseal = append(m.onUnseal, f)
}

func (m *MarketEvent) ResponseMarketEvent(ctx context.Context, resp *gtypes.ResponseEvent) error {
	url, err := m.recorder.Get(ctx, resp.ID)
	if err != nil {
		return err
	}
	client, ok := m.clients[url]
	if !ok {
		return fmt.Errorf("client not found for url %s", url)
	}
	// rm record
	err = m.recorder.Remove(ctx, resp.ID)
	if err != nil {
		log.Errorf("remove event record failed: %s", err)
	}
	return client.ResponseMarketEvent(ctx, resp)
}

func (m *MarketEvent) RespondUnseal(ctx context.Context, eventID types.UUID, err error) error {
	return m.ResponseMarketEvent(ctx, &gtypes.ResponseEvent{
		ID:      eventID,
		Payload: nil,
		Error:   err.Error(),
	})
}

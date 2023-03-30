package market

import (
	"context"

	"time"

	"encoding/json"

	"github.com/filecoin-project/go-address"
	gateway "github.com/filecoin-project/venus/venus-shared/api/gateway/v1"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"
)

// MarketEventClient is a client connect to gateway to listen market event
// a client can listen for many miner, and corresponds to the gateway one by one
type MarketEventClient struct {
	gateway.IMarketServiceProvider
	url string
}

func (m *MarketEventClient) ListenOnMiner(ctx context.Context, miner address.Address, reqCh chan<- *gtypes.RequestEvent) {
	var eventCh <-chan *gtypes.RequestEvent
	var err error
	for {
		eventCh, err = m.ListenMarketEvent(ctx, &gtypes.MarketRegisterPolicy{
			Miner: miner,
		})
		if err == nil {
			break
		}
		// try to reconnect
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second * 5):
		}
	}

	// check init event
	select {
	case <-ctx.Done():
		return
	case event := <-eventCh:
		if event == nil {
			log.Error("odd error in connect: nil event")
		}
		switch event.Method {
		case "InitConnect":
			req := gtypes.ConnectedCompleted{}
			err := json.Unmarshal(event.Payload, &req)
			if err != nil {
				log.Errorf("odd error in connect %v", err)
			}
		default:
			log.Error("odd error in connect : not init before " + event.Method)
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e := <-eventCh:
				switch e.Method {
				case "IsUnsealed", "SectorsUnsealPiece":
					reqCh <- e
				default:
					log.Errorf("%s receive unexpected market event type %s", miner, e.Method)
				}
			}
		}
	}()
}

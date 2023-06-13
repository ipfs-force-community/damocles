package market

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/api/gateway/v2"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("market_event")

type Event struct {
	unseal core.UnsealSectorManager
	client gateway.IMarketServiceProvider
	miner  address.Address
}

func New(unseal core.UnsealSectorManager, client gateway.IMarketServiceProvider, miner address.Address) *Event {
	return &Event{unseal: unseal, client: client, miner: miner}
}

func (me *Event) StartListening(ctx context.Context) {
	log.Infof("start market event listening for %s", me.miner)
	for {
		if err := me.listenMarketRequestOnce(ctx); err != nil {
			log.Errorf("%s listen market event errored: %s", me.miner, err)
		} else {
			log.Warnf(" %s listen market event quit", me.miner)
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			log.Warnf("%s not restarting listen market event: context error: %s", me.miner, ctx.Err())
			return
		}

		log.Infof("restarting listen market event for %s", me.miner)
	}
}

func (me *Event) listenMarketRequestOnce(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	policy := &gtypes.MarketRegisterPolicy{
		Miner: me.miner,
	}

	marketEventCh, err := me.client.ListenMarketEvent(ctx, policy)
	if err != nil {
		// Retry is handled by caller
		return fmt.Errorf("listenmarketChanges ChainNotify call failed: %w", err)
	}

	for event := range marketEventCh {
		switch event.Method {
		case "InitConnect":
			req := gtypes.ConnectedCompleted{}
			err := json.Unmarshal(event.Payload, &req)
			if err != nil {
				return fmt.Errorf("odd error in connect %v", err)
			}
			log.Infof("%s success to connect with market %s", me.miner, req.ChannelId)
		case "SectorsUnsealPiece":
			respondError := func(err error) {
				rErr := me.client.ResponseMarketEvent(ctx, &gtypes.ResponseEvent{
					ID:      event.ID,
					Payload: nil,
					Error:   err.Error(),
				})
				log.Errorf("response event %s failed: %s", event.ID, rErr)
			}

			req := gtypes.UnsealRequest{}
			err := json.Unmarshal(event.Payload, &req)
			if err != nil {
				respondError(err)
				continue
			}
			// to unseal the request
			actor, err := address.IDFromAddress(req.Miner)
			if err != nil {
				log.Errorf("get miner id from address: %s", err)
				respondError(err)
				continue
			}
			info := &core.SectorUnsealInfo{
				Sector: core.AllocatedSector{
					ID: abi.SectorID{
						Miner:  abi.ActorID(actor),
						Number: req.Sid,
					},
				},
				PieceCid: req.PieceCid,
				Offset:   req.Offset,
				Size:     req.Size,
				Dest:     []string{req.Dest},
			}
			state, err := me.unseal.SetAndCheck(ctx, info)
			if err != nil {
				log.Errorf("set unseal info: %s", err)
				respondError(err)
				continue
			}
			stateBytes, err := json.Marshal(state)
			if err != nil {
				respondError(err)
				continue
			}
			err = me.client.ResponseMarketEvent(ctx, &gtypes.ResponseEvent{
				ID:      event.ID,
				Payload: stateBytes,
			})
			if err != nil {
				log.Errorf("response event %s failed: %s", event.ID, err)
			}

		default:
			log.Errorf("%s receive unexpected market event type %s", me.miner, event.Method)
		}
	}

	return nil
}

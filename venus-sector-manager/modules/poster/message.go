package poster

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"

	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/types"
	mtypes "github.com/filecoin-project/venus/venus-shared/types/messager"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

type msgOrErr struct {
	*mtypes.Message
	err error
}

type diPeriod struct {
	index uint64
	open  abi.ChainEpoch
}

func (s *scheduler) publishMessage(ctx context.Context, method abi.MethodNum, params cbor.Marshaler, di *diPeriod, wait bool) (string, <-chan msgOrErr, error) {
	mcfg, err := s.cfg.MinerConfig(s.actor.ID)
	if err != nil {
		return "", nil, fmt.Errorf("get sender: %w", err)
	}

	encoded, err := actors.SerializeParams(params)
	if err != nil {
		return "", nil, fmt.Errorf("serialize params: %w", err)
	}

	msg := types.Message{
		From:   mcfg.PoSt.Sender.Std(),
		To:     s.actor.Addr,
		Method: method,
		Params: encoded,
		Value:  types.NewInt(0),
	}

	spec := messager.MsgMeta{
		GasOverEstimation: mcfg.PoSt.GasOverEstimation,
		MaxFeeCap:         mcfg.PoSt.MaxFeeCap.Std(),
	}

	mid := ""
	if di == nil {
		mid = msg.Cid().String()
	} else {
		mid = fmt.Sprintf("%s-%v-%v", msg.Cid().String(), di.index, di.open)
	}
	uid, err := s.msg.PushMessageWithId(ctx, mid, &msg, &spec)
	if err != nil {
		return "", nil, fmt.Errorf("push msg with id %s: %w", mid, err)
	}

	if !wait {
		return uid, nil, nil
	}

	ch := make(chan msgOrErr, 1)
	go func() {
		defer close(ch)

		m, err := s.waitMessage(ctx, uid, mcfg.PoSt.Confidence)
		ch <- msgOrErr{
			Message: m,
			err:     err,
		}
	}()

	return uid, ch, nil
}

func (s *scheduler) waitMessage(ctx context.Context, mid string, confidence uint64) (*mtypes.Message, error) {
	return s.msg.WaitMessage(ctx, mid, confidence)
}

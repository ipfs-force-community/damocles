package poster

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	mtypes "github.com/filecoin-project/venus-messager/types"
	"github.com/filecoin-project/venus/pkg/specactors"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

type msgOrErr struct {
	*mtypes.Message
	err error
}

func (s *scheduler) publishMessage(ctx context.Context, method abi.MethodNum, params cbor.Marshaler, wait bool) (string, <-chan msgOrErr, error) {
	sender, err := senderFromConfig(s.actor.ID, s.cfg)
	if err != nil {
		return "", nil, fmt.Errorf("get sender: %w", err)
	}

	policy := postPolicyFromConfig(s.actor.ID, s.cfg)

	encoded, err := specactors.SerializeParams(params)
	if err != nil {
		return "", nil, fmt.Errorf("serialize params: %w", err)
	}

	msg := types.Message{
		From:   sender,
		To:     s.actor.Addr,
		Params: encoded,
		Value:  types.NewInt(0),
	}

	mcid := msg.Cid()

	spec := messager.MsgMeta{
		GasOverEstimation: policy.GasOverEstimation,
		MaxFeeCap:         policy.MaxFeeCap.Std(),
	}

	uid, err := s.msg.PushMessageWithId(ctx, mcid.String(), &msg, &spec)
	if err != nil {
		return "", nil, fmt.Errorf("push msg with id %s: %w", mcid, err)
	}

	if !wait {
		return uid, nil, nil
	}

	ch := make(chan msgOrErr, 1)
	go func() {
		defer close(ch)

		m, err := s.waitMessage(ctx, uid, policy.MsgConfidence)
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

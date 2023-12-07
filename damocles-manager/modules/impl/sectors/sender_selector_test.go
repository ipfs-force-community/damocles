package sectors_test

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/sectors"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
)

func TestSelectSender(t *testing.T) {

	balances := map[address.Address]big.Int{
		lo.Must(address.NewIDAddress(1000)): big.NewInt(1000),
		lo.Must(address.NewIDAddress(2000)): big.NewInt(2000),
		lo.Must(address.NewIDAddress(3000)): big.NewInt(3000),
		lo.Must(address.NewIDAddress(4000)): big.NewInt(4000),
		lo.Must(address.NewIDAddress(5000)): big.NewInt(5000),
	}

	var mockChain chain.MockStruct

	policy.NetParams = &types.NetworkParams{
		BlockDelaySecs: 30,
	}

	mockChain.IMinerStateStruct.Internal.StateMinerInfo = func(_ctx context.Context, _maddr address.Address, _tsk types.TipSetKey) (types.MinerInfo, error) {
		return types.MinerInfo{
			Owner:            lo.Must(address.NewIDAddress(1000)),
			Worker:           lo.Must(address.NewIDAddress(2000)),
			ControlAddresses: []address.Address{lo.Must(address.NewIDAddress(3000)), lo.Must(address.NewIDAddress(4000)), lo.Must(address.NewIDAddress(5000))},
		}, nil
	}

	mockChain.IActorStruct.Internal.StateGetActor = func(_ctx context.Context, actor address.Address, _tsk types.TipSetKey) (*types.Actor, error) {
		return &types.Actor{
			Balance: balances[actor],
		}, nil
	}

	ctx := context.TODO()

	sel := sectors.NewSenderSelector(&mockChain)
	senders := []address.Address{lo.Must(address.NewIDAddress(2000)), lo.Must(address.NewIDAddress(9999)), lo.Must(address.NewIDAddress(5000)), lo.Must(address.NewIDAddress(1000))}
	sender, err := sel.Select(ctx, 1, senders)
	require.NoError(t, err)
	require.Equal(t, lo.Must(address.NewIDAddress(5000)).String(), sender.String())
}

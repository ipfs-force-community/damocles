package policy

import (
	"context"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/filecoin-project/venus/venus-shared/utils"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("policy")

var NetParams *types.NetworkParams

func SetupNetwork(ctx context.Context, api chain.API) (err error) {
	NetParams, err = api.StateGetNetworkParams(ctx)
	if err != nil {
		return
	}

	err = utils.LoadBuiltinActors(ctx, api)
	if err != nil {
		return
	}

	log.Infow("NETWORK SETUP", "name", NetParams.NetworkName)
	return
}

const (
	EpochsInDay = builtin.EpochsInDay

	ChainFinality                  = policy.ChainFinality
	SealRandomnessLookback         = ChainFinality
	MaxPreCommitRandomnessLookback = builtin.EpochsInDay + SealRandomnessLookback
)

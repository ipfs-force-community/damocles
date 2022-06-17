package policy

import (
	"fmt"

	"github.com/filecoin-project/venus/fixtures/networks"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/policy"
	builtinactors "github.com/filecoin-project/venus/venus-shared/builtin-actors"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("policy")

var NetParams *networks.NetworkConf

func SetupNetwork(name string) error {
	log.Debugw("NETWORK SETUP", "name", name)
	var networkType types.NetworkType
	switch name {
	case "mainnet":
		NetParams = networks.Mainnet()
		networkType = types.NetworkMainnet
	case "integrationnet":
		NetParams = networks.IntegrationNet()
		networkType = types.Integrationnet
	case "2k":
		NetParams = networks.Net2k()
		networkType = types.Network2k
	case "cali":
		NetParams = networks.Calibration()
		networkType = types.NetworkCalibnet
	case "interop":
		NetParams = networks.InteropNet()
		networkType = types.NetworkInterop
	case "force":
		NetParams = networks.ForceNet()
		networkType = types.NetworkForce
	case "butterfly":
		NetParams = networks.ButterflySnapNet()
		networkType = types.NetworkButterfly
	default:
		return fmt.Errorf("invalid network name %s", name)
	}

	return builtinactors.SetNetworkBundle(networkType)
}

const (
	EpochsInDay = builtin.EpochsInDay

	ChainFinality                  = policy.ChainFinality
	SealRandomnessLookback         = ChainFinality
	MaxPreCommitRandomnessLookback = builtin.EpochsInDay + SealRandomnessLookback
)

package policy

import (
	"fmt"

	"github.com/filecoin-project/venus/fixtures/networks"
	"github.com/filecoin-project/venus/pkg/specactors/builtin"
	"github.com/filecoin-project/venus/pkg/specactors/policy"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("policy")

var NetParams *networks.NetworkConf

func SetupNetwork(name string) error {
	log.Debugw("NETWORK SETUP", "name", name)
	switch name {
	case "mainnet":
		NetParams = networks.Mainnet()
	case "nerpa":
		NetParams = networks.NerpaNet()
	case "testnetnet":
		NetParams = networks.Testnet()
	case "integrationnet":
		NetParams = networks.IntegrationNet()
	case "2k":
		NetParams = networks.Net2k()
	case "cali":
		NetParams = networks.Calibration()
	case "interop":
		NetParams = networks.InteropNet()
	default:
		return fmt.Errorf("invalid network name %s", name)
	}

	return nil
}

const (
	ChainFinality                  = policy.ChainFinality
	SealRandomnessLookback         = ChainFinality
	MaxPreCommitRandomnessLookback = builtin.EpochsInDay + SealRandomnessLookback
)

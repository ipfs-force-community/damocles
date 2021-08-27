package commitmgr

import (
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
)

type Cfg struct {
	*modules.Config
	confmgr.RLocker
}

func (c *Cfg) policy(mid abi.ActorID) modules.CommitmentPolicyConfig {
	c.Lock()
	defer c.Unlock()

	return c.Config.Commitment.MustPolicy(mid)
}

func (c *Cfg) ctrl(mid abi.ActorID) (modules.CommitmentControlAddress, bool) {
	c.Lock()
	defer c.Unlock()

	cmcfg, ok := c.Commitment.Miners[modules.ActorID2ConfigKey(mid)]
	if !ok {
		return modules.CommitmentControlAddress{}, false
	}

	return cmcfg.Controls, true
}

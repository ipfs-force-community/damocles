package commitmgr

import (
	"strconv"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sector-manager/modules"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/confmgr"
)

type Cfg struct {
	*modules.Config
	confmgr.RLocker
}

func (c *Cfg) policy(mid abi.ActorID) modules.CommitmentPolicyConfig {
	c.Lock()
	defer c.Unlock()

	key := strconv.FormatUint(uint64(mid), 10)
	return c.Config.Commitment.MustPolicy(key)
}

func (c *Cfg) ctrl(mid abi.ActorID) (modules.CommitmentControlAddress, bool) {
	c.Lock()
	defer c.Unlock()

	cmcfg, ok := c.Commitment.Miners[strconv.FormatUint(uint64(mid), 10)]
	if !ok {
		return modules.CommitmentControlAddress{}, false
	}

	return cmcfg.Controls, true
}

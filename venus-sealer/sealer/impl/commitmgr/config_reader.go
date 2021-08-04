package commitmgr

import (
	"strconv"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
)

type Cfg struct {
	*sealer.Config
	confmgr.RLocker
}

func (c *Cfg) policy(mid abi.ActorID) sealer.CommitmentPolicyConfig {
	c.Lock()
	defer c.Unlock()

	key := strconv.FormatUint(uint64(mid), 10)
	return c.Config.Commitment.MustPolicy(key)
}

func (c *Cfg) ctrl(mid abi.ActorID) (sealer.CommitmentControlAddress, bool) {
	c.Lock()
	defer c.Unlock()

	cmcfg, ok := c.Commitment.Miners[strconv.FormatUint(uint64(mid), 10)]
	if !ok {
		return sealer.CommitmentControlAddress{}, false
	}

	return cmcfg.Controls, true
}

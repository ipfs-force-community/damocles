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
	cmcfg, ok := c.Commitment[key]
	if !ok {
		cmcfg = c.Commitment[sealer.DefaultCommitmentKey]
	}

	return cmcfg.CommitmentPolicyConfig
}

func (c *Cfg) ctrl(mid abi.ActorID) *sealer.CommitmentControlAddress {
	c.Lock()
	defer c.Unlock()

	return c.Commitment[strconv.FormatUint(uint64(mid), 10)].Control
}

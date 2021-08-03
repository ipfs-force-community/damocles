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

func (c *Cfg) commitment(mid abi.ActorID) sealer.CommitmentManagerConfig {
	c.Lock()
	defer c.Unlock()
	key := strconv.FormatUint(uint64(mid), 10)
	cmcfg, ok := c.Commitment[key]
	if ok {
		return cmcfg
	}

	cmcfg = c.Commitment[sealer.DefaultCommitmentKey]
	return cmcfg
}

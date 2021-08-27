package poster

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sector-manager/modules"
)

func postPolicyFromConfig(mid abi.ActorID, cfg *modules.SafeConfig) modules.PoStPolicyConfig {
	cfg.Lock()
	pcfg := cfg.PoSt.MustPolicy(mid)
	cfg.Unlock()
	return pcfg
}

func senderFromConfig(mid abi.ActorID, cfg *modules.SafeConfig) (address.Address, error) {
	cfg.Lock()
	pcfg, ok := cfg.PoSt.Actors[modules.ActorID2ConfigKey(mid)]
	cfg.Unlock()

	if !ok {
		return address.Undef, fmt.Errorf("sender not found for actor %d", mid)
	}

	return pcfg.Sender.Std(), nil
}

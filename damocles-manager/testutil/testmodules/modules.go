package testmodules

import (
	"sync"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
)

var TestActorBase abi.ActorID = 10000

func MockSafeConfig(count int, minerInitializer func(mcfg *modules.MinerConfig)) (*modules.SafeConfig, sync.Locker) {
	cfg := modules.DefaultConfig(false)
	for i := 0; i < count; i++ {
		mcfg := modules.DefaultMinerConfig(false)
		mcfg.Actor = TestActorBase + abi.ActorID(i)
		if minerInitializer != nil {
			minerInitializer(&mcfg)
		}

		cfg.Miners = append(cfg.Miners, mcfg)
	}

	var lock sync.RWMutex

	return &modules.SafeConfig{
		Config: &cfg,
		Locker: lock.RLocker(),
	}, &lock
}

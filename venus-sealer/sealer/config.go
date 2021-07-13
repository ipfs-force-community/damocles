package sealer

import "github.com/filecoin-project/go-state-types/abi"

const ConfigKey = "sealer"

func DefaultConfig() Config {
	return Config{
		SectorManager: DefaultSectorManagerConfig(),
	}
}

type Config struct {
	SectorManager SectorManagerConfig
}

func DefaultSectorManagerConfig() SectorManagerConfig {
	return SectorManagerConfig{
		Miners: []abi.ActorID{},
	}
}

type SectorManagerConfig struct {
	Miners []abi.ActorID
}

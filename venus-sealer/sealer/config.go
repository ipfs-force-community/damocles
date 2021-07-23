package sealer

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

const ConfigKey = "sealer"

func DefaultConfig() Config {
	return Config{
		SectorManager:     DefaultSectorManagerConfig(),
		CommitmentManager: DefaultCommitmentManagerConfig(),
		MessagerClient:    DefaultMessagerClientConfig(),
	}
}

type Config struct {
	SectorManager     SectorManagerConfig
	CommitmentManager map[address.Address]CommitmentManagerConfig
	MessagerClient    MessagerClientConfig
}

func DefaultSectorManagerConfig() SectorManagerConfig {
	return SectorManagerConfig{
		Miners: []abi.ActorID{},
	}
}

type SectorManagerConfig struct {
	Miners []abi.ActorID
}

type CommitmentManagerConfig struct {
	CommitBatchThreshold int
	CommitBatchMaxWait   time.Duration
	CommitCheckInterval  time.Duration
	EnableBatchProCommit bool

	PreCommitBatchThreshold int
	PreCommitBatchMaxWait   time.Duration
	PreCommitCheckInterval  time.Duration
	EnableBatchPreCommit    bool

	PreCommitGasOverEstimation      float64
	ProCommitGasOverEstimation      float64
	BatchPreCommitGasOverEstimation float64
	BatchProCommitGasOverEstimation float64

	MaxPreCommitFeeCap      big.Int
	MaxProCommitFeeCap      big.Int
	MaxBatchPreCommitFeeCap big.Int
	MaxBatchProCommitFeeCap big.Int

	PreCommitControlAddress string
	ProCommitControlAddress string

	MsgConfidence int64
}

func DefaultCommitmentManagerConfig() map[address.Address]CommitmentManagerConfig {
	return map[address.Address]CommitmentManagerConfig{}
}

type MessagerClientConfig struct {
	Api   string
	Token string
}

func DefaultMessagerClientConfig() MessagerClientConfig {
	return MessagerClientConfig{
		Api:   "",
		Token: "",
	}
}

package sealer

import (
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

const ConfigKey = "sealer"
const DefaultCommitmentKey = "default"

func DefaultConfig() Config {
	return Config{
		SectorManager: DefaultSectorManagerConfig(),
		Commitment:    DefaultCommitment(),
		Chain:         RPCClientConfig{},
		Messager:      RPCClientConfig{},
	}
}

type Config struct {
	SectorManager SectorManagerConfig
	Commitment    map[string]CommitmentManagerConfig
	Chain         RPCClientConfig
	Messager      RPCClientConfig
}

func DefaultSectorManagerConfig() SectorManagerConfig {
	return SectorManagerConfig{
		Miners: []abi.ActorID{},
	}
}

type SectorManagerConfig struct {
	Miners []abi.ActorID
}

type CommitmentPolicyConfig struct {
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
	MsgConfidence           int64
}

type CommitmentControlAddress struct {
	PreCommit   address.Address
	ProveCommit address.Address
}

type CommitmentManagerConfig struct {
	Control *CommitmentControlAddress
	CommitmentPolicyConfig
}

func DefaultCommitmentManagerConfig() CommitmentManagerConfig {
	return CommitmentManagerConfig{}
}

func DefaultCommitment() map[string]CommitmentManagerConfig {
	return map[string]CommitmentManagerConfig{
		DefaultCommitmentKey: CommitmentManagerConfig{},
	}
}

type RPCClientConfig struct {
	Api   string
	Token string
}

package sealer

import (
	"time"

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
	CommitmentManager CommitmentManagerConfig
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

	CheaperBaseFee abi.TokenAmount

	PreCommitGasOverEstimation      float64
	ProCommitGasOverEstimation      float64
	BatchPreCommitGasOverEstimation float64
	BatchProCommitGasOverEstimation float64

	MaxPreCommitFeeCap      big.Int
	MaxProCommitFeeCap      big.Int
	MaxBatchPreCommitFeeCap big.Int
	MaxBatchProCommitFeeCap big.Int

	PreCommitControlAddress map[string]string
	ProCommitControlAddress map[string]string

	MsgConfidence int64
}

func DefaultCommitmentManagerConfig() CommitmentManagerConfig {
	return CommitmentManagerConfig{
		CommitBatchThreshold:    0,
		PreCommitBatchThreshold: 0,
		CommitBatchMaxWait:      0,

		CheaperBaseFee:                  abi.TokenAmount{},
		PreCommitGasOverEstimation:      0,
		ProCommitGasOverEstimation:      0,
		BatchPreCommitGasOverEstimation: 0,
		BatchProCommitGasOverEstimation: 0,
		MaxPreCommitFeeCap:              big.Int{},
		MaxProCommitFeeCap:              big.Int{},
		MaxBatchPreCommitFeeCap:         big.Int{},
		MaxBatchProCommitFeeCap:         big.Int{},
		EnableBatchPreCommit:            false,
		EnableBatchProCommit:            false,
		PreCommitControlAddress:         nil,
		ProCommitControlAddress:         nil,
		MsgConfidence:                   0,
	}
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

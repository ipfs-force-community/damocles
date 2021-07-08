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
	}
}

type Config struct {
	SectorManager     SectorManagerConfig
	CommitmentManager CommitmentManagerConfig
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
	MaxCommitBatch   int
	MinCommitBatch   int
	CommitBatchWait  time.Duration
	CommitBatchSlack time.Duration

	MaxPreCommitBatch   int
	MinPreCommitBatch   int
	PreCommitBatchWait  time.Duration
	PreCommitBatchSlack time.Duration

	CheaperBaseFee abi.TokenAmount

	PreCommitGasOverEstimation      float64
	ProCommitGasOverEstimation      float64
	BatchPreCommitGasOverEstimation float64
	BatchProCommitGasOverEstimation float64

	MaxPreCommitFeeCap      big.Int
	MaxProCommitFeeCap      big.Int
	MaxBatchPreCommitFeeCap big.Int
	MaxBatchProCommitFeeCap big.Int

	EnableBatchPreCommit bool
	EnableBatchProCommit bool

	PreCommitControlAddress map[string]string
	ProCommitControlAddress map[string]string

	MsgConfidence int64
}

func DefaultCommitmentManagerConfig() CommitmentManagerConfig {
	return CommitmentManagerConfig{
		MaxCommitBatch:                  0,
		MinCommitBatch:                  0,
		CommitBatchWait:                 0,
		CommitBatchSlack:                0,
		MaxPreCommitBatch:               0,
		MinPreCommitBatch:               0,
		PreCommitBatchWait:              0,
		PreCommitBatchSlack:             0,
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

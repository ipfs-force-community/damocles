package sealer

import (
	"bytes"
	"reflect"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
)

const ConfigKey = "sealer"
const DefaultCommitmentKey = "default"

func init() {
	checkOptionalConfig(reflect.TypeOf(CommitmentPolicyConfig{}), reflect.TypeOf(CommitmentPolicyConfigOptional{}))
}

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
	Commitment    CommitmentManagerConfig
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

func DefaultCommitment() CommitmentManagerConfig {
	return CommitmentManagerConfig{
		DefaultPolicy: DefaultCommitmentPolicy(),
		Miners:        map[string]CommitmentMinerConfig{},
	}
}

type CommitmentManagerConfig struct {
	DefaultPolicy CommitmentPolicyConfig
	Miners        map[string]CommitmentMinerConfig
}

func (c *CommitmentManagerConfig) MustPolicy(key string) CommitmentPolicyConfig {
	cfg, err := c.Policy(key)
	if err != nil {
		panic(err)
	}

	return cfg
}

func (c *CommitmentManagerConfig) Policy(key string) (CommitmentPolicyConfig, error) {
	var buf bytes.Buffer
	err := toml.NewEncoder(&buf).Encode(c.DefaultPolicy)
	if err != nil {
		return CommitmentPolicyConfig{}, err
	}

	var cloned CommitmentPolicyConfig
	_, err = toml.Decode(string(buf.Bytes()), &cloned)
	if err != nil {
		return CommitmentPolicyConfig{}, err
	}

	if opt, ok := c.Miners[key]; ok {
		buf.Reset()
		err = toml.NewEncoder(&buf).Encode(opt.CommitmentPolicyConfigOptional)
		if err != nil {
			return CommitmentPolicyConfig{}, err
		}

		_, err = toml.Decode(string(buf.Bytes()), &cloned)
		if err != nil {
			return CommitmentPolicyConfig{}, err
		}
	}

	return cloned, nil
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

func DefaultCommitmentPolicy() CommitmentPolicyConfig {
	return CommitmentPolicyConfig{}

}

type CommitmentPolicyConfigOptional struct {
	CommitBatchThreshold *int
	CommitBatchMaxWait   *time.Duration
	CommitCheckInterval  *time.Duration
	EnableBatchProCommit *bool

	PreCommitBatchThreshold *int
	PreCommitBatchMaxWait   *time.Duration
	PreCommitCheckInterval  *time.Duration
	EnableBatchPreCommit    *bool

	PreCommitGasOverEstimation      *float64
	ProCommitGasOverEstimation      *float64
	BatchPreCommitGasOverEstimation *float64
	BatchProCommitGasOverEstimation *float64

	MaxPreCommitFeeCap      *big.Int
	MaxProCommitFeeCap      *big.Int
	MaxBatchPreCommitFeeCap *big.Int
	MaxBatchProCommitFeeCap *big.Int
	MsgConfidence           *int64
}

type CommitmentControlAddress struct {
	PreCommit   address.Address
	ProveCommit address.Address
}

type CommitmentMinerConfig struct {
	Controls CommitmentControlAddress
	CommitmentPolicyConfigOptional
}

type RPCClientConfig struct {
	Api   string
	Token string
}

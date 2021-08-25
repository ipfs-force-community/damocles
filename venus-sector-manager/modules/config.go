package modules

import (
	"bytes"
	"reflect"

	"github.com/BurntSushi/toml"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
	"github.com/filecoin-project/go-state-types/abi"
)

const ConfigKey = "sector-manager"

func init() {
	checkOptionalConfig(reflect.TypeOf(CommitmentPolicyConfig{}), reflect.TypeOf(CommitmentPolicyConfigOptional{}))
}

func DefaultConfig() Config {
	return Config{
		SectorManager: DefaultSectorManagerConfig(),
		Commitment:    DefaultCommitment(),
		Chain:         RPCClientConfig{},
		Messager:      RPCClientConfig{},
		PersistedStore: FileStoreConfig{
			Includes: make([]string, 0),
			Stores:   make([]filestore.Config, 0),
		},
	}
}

type Config struct {
	SectorManager  SectorManagerConfig
	Commitment     CommitmentManagerConfig
	Chain          RPCClientConfig
	Messager       RPCClientConfig
	PersistedStore FileStoreConfig
}

func DefaultSectorManagerConfig() SectorManagerConfig {
	return SectorManagerConfig{
		Miners:   []abi.ActorID{},
		PreFetch: true,
	}
}

type SectorManagerConfig struct {
	Miners   []abi.ActorID
	PreFetch bool
}

func DefaultCommitment() CommitmentManagerConfig {
	return CommitmentManagerConfig{
		DefaultPolicy: DefaultCommitmentPolicy(),
		Miners: map[string]CommitmentMinerConfig{
			"example": CommitmentMinerConfig{},
		},
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
	CommitBatchMaxWait   Duration
	CommitCheckInterval  Duration
	EnableBatchProCommit bool

	PreCommitBatchThreshold int
	PreCommitBatchMaxWait   Duration
	PreCommitCheckInterval  Duration
	EnableBatchPreCommit    bool

	PreCommitGasOverEstimation      float64
	ProCommitGasOverEstimation      float64
	BatchPreCommitGasOverEstimation float64
	BatchProCommitGasOverEstimation float64

	MaxPreCommitFeeCap      BigInt
	MaxProCommitFeeCap      BigInt
	MaxBatchPreCommitFeeCap BigInt
	MaxBatchProCommitFeeCap BigInt
	MsgConfidence           int64
}

func DefaultCommitmentPolicy() CommitmentPolicyConfig {
	return CommitmentPolicyConfig{}

}

type CommitmentPolicyConfigOptional struct {
	CommitBatchThreshold *int
	CommitBatchMaxWait   *Duration
	CommitCheckInterval  *Duration
	EnableBatchProCommit *bool

	PreCommitBatchThreshold *int
	PreCommitBatchMaxWait   *Duration
	PreCommitCheckInterval  *Duration
	EnableBatchPreCommit    *bool

	PreCommitGasOverEstimation      *float64
	ProCommitGasOverEstimation      *float64
	BatchPreCommitGasOverEstimation *float64
	BatchProCommitGasOverEstimation *float64

	MaxPreCommitFeeCap      *BigInt
	MaxProCommitFeeCap      *BigInt
	MaxBatchPreCommitFeeCap *BigInt
	MaxBatchProCommitFeeCap *BigInt
	MsgConfidence           *int64
}

type CommitmentControlAddress struct {
	PreCommit   MustAddress
	ProveCommit MustAddress
}

type CommitmentMinerConfig struct {
	Controls CommitmentControlAddress
	CommitmentPolicyConfigOptional
}

type RPCClientConfig struct {
	Api   string
	Token string
}

type FileStoreConfig struct {
	Includes []string
	Stores   []filestore.Config
}

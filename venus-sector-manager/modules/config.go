package modules

import (
	"reflect"
	"sync"
	"time"

	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
	"github.com/filecoin-project/go-state-types/abi"
)

const ConfigKey = "sector-manager"

func init() {
	checkOptionalConfig(reflect.TypeOf(CommitmentPolicyConfig{}), reflect.TypeOf(CommitmentPolicyConfigOptional{}))
	checkOptionalConfig(reflect.TypeOf(PoStPolicyConfig{}), reflect.TypeOf(PoStPolicyConfigOptional{}))
}

type SafeConfig struct {
	*Config
	sync.Locker
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
		PoSt: DefaultPoStConfig(),
	}
}

type Config struct {
	SectorManager  SectorManagerConfig
	Commitment     CommitmentManagerConfig
	Chain          RPCClientConfig
	Messager       RPCClientConfig
	PersistedStore FileStoreConfig
	// TODO: use separate config for each actor
	PoSt PoStConfig
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

func (c *CommitmentManagerConfig) MustPolicy(mid abi.ActorID) CommitmentPolicyConfig {
	cfg, err := c.Policy(mid)
	if err != nil {
		panic(err)
	}

	return cfg
}

func (c *CommitmentManagerConfig) Policy(mid abi.ActorID) (CommitmentPolicyConfig, error) {
	var dest CommitmentPolicyConfig
	var optional interface{}

	if opt, ok := c.Miners[ActorID2ConfigKey(mid)]; ok {
		optional = opt.CommitmentPolicyConfigOptional
	}

	err := cloneConfig(&dest, c.DefaultPolicy, optional)
	if err != nil {
		return CommitmentPolicyConfig{}, err
	}

	return dest, nil
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

func DefaultPoStConfig() PoStConfig {
	return PoStConfig{
		Default: PoStPolicyConfig{
			MsgCheckInteval: Duration(time.Minute),
			MsgConfidence:   5,
		},
		Actors: map[string]PoStActorConfig{},
	}
}

type PoStConfig struct {
	Default PoStPolicyConfig
	Actors  map[string]PoStActorConfig
}

func (c *PoStConfig) MustPolicy(mid abi.ActorID) PoStPolicyConfig {
	cfg, err := c.Policy(mid)
	if err != nil {
		panic(err)
	}

	return cfg
}

func (c *PoStConfig) Policy(mid abi.ActorID) (PoStPolicyConfig, error) {
	var dest PoStPolicyConfig
	var optional interface{}

	if opt, ok := c.Actors[ActorID2ConfigKey(mid)]; ok {
		optional = opt.PoStPolicyConfigOptional
	}

	err := cloneConfig(&dest, c.Default, optional)
	if err != nil {
		return PoStPolicyConfig{}, err
	}

	return dest, nil
}

type PoStPolicyConfig struct {
	StrictCheck       bool
	GasOverEstimation float64
	MaxFeeCap         BigInt

	MsgCheckInteval Duration
	MsgConfidence   uint64
}

type PoStPolicyConfigOptional struct {
	StrictCheck       *bool
	GasOverEstimation *float64
	MaxFeeCap         *BigInt

	MsgCheckInteval *Duration
	MsgConfidence   *uint64
}

type PoStActorConfig struct {
	Sender MustAddress
	PoStPolicyConfigOptional
}

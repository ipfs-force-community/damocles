package modules

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
)

func init() {
	fake, err := address.NewFromString("f1abjxfbp274xpdqcpuaykwkfb43omjotacm2p3za")
	if err != nil {
		panic(fmt.Errorf("parse fake address: %w", err))
	}

	fakeAddress = MustAddress(fake)
}

var fakeAddress MustAddress

const ConfigKey = "sector-manager"

func init() {
	checkOptionalConfig(reflect.TypeOf(CommitmentPolicyConfig{}), reflect.TypeOf(CommitmentPolicyConfigOptional{}))
	checkOptionalConfig(reflect.TypeOf(PoStPolicyConfig{}), reflect.TypeOf(PoStPolicyConfigOptional{}))
}

type SafeConfig struct {
	*Config
	sync.Locker
}

func ExampleConfig() Config {
	defaultCfg := DefaultConfig()

	// Example for miner section
	var maxNumber uint64 = 10000
	defaultCfg.SectorManager.Miners = append(defaultCfg.SectorManager.Miners, SectorManagerMinerConfig{
		ID:         10000,
		InitNumber: 0,
		MaxNumber:  &maxNumber,
		Disabled:   false,
	})
	defaultCfg.Commitment.Miners["example"] = ExampleCommitmentMinerConfig()
	defaultCfg.PersistedStore.Includes = append(defaultCfg.PersistedStore.Includes, "unavailable")
	defaultCfg.PersistedStore.Stores = append(defaultCfg.PersistedStore.Stores, filestore.Config{
		Name:     "storage name,like `100.100.10.1`",
		Path:     "/path/to/storage/",
		Strict:   false,
		ReadOnly: true,
	})

	defaultCfg.PoSt.Actors["10000"] = PoStActorConfig{
		Sender: fakeAddress,
		PoStPolicyConfigOptional: PoStPolicyConfigOptional{
			StrictCheck:       &defaultCfg.PoSt.Default.StrictCheck,
			GasOverEstimation: &defaultCfg.PoSt.Default.GasOverEstimation,
			MaxFeeCap:         &defaultCfg.PoSt.Default.MaxFeeCap,
			MsgCheckInteval:   &defaultCfg.PoSt.Default.MsgCheckInteval,
			MsgConfidence:     &defaultCfg.PoSt.Default.MsgConfidence,
		},
	}

	defaultCfg.RegisterProof.Actors["10000"] = RegisterProofActorConfig{
		Apis:  []string{},
		Token: "",
	}

	defaultCfg.DealManagerConfig.Enable = true
	defaultCfg.DealManagerConfig.MarketAPI.Api = "/ip4/{host}/tcp/{port}"
	defaultCfg.DealManagerConfig.MarketAPI.Token = "some token"
	defaultCfg.DealManagerConfig.LocalPieceStores = append(defaultCfg.DealManagerConfig.LocalPieceStores, filestore.Config{
		Name:     "piece storage name",
		Path:     "/path/to/piece/storage",
		Strict:   true,
		ReadOnly: true,
	})

	return defaultCfg
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
		PoSt:              DefaultPoStConfig(),
		RegisterProof:     DefaultRegisterProofConfig(),
		DealManagerConfig: DefaultDealManagerConfig(),
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

	RegisterProof     RegisterProofConfig
	DealManagerConfig DealManagerConfig
}

func DefaultSectorManagerConfig() SectorManagerConfig {
	return SectorManagerConfig{
		Miners:   []SectorManagerMinerConfig{},
		PreFetch: true,
	}
}

type SectorManagerConfig struct {
	Miners   []SectorManagerMinerConfig
	PreFetch bool
}

type SectorManagerMinerConfig struct {
	ID         abi.ActorID
	InitNumber uint64
	MaxNumber  *uint64
	Disabled   bool
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
	return CommitmentPolicyConfig{
		CommitBatchThreshold:            16,
		CommitBatchMaxWait:              Duration(time.Hour),
		CommitCheckInterval:             Duration(time.Minute),
		PreCommitBatchThreshold:         16,
		PreCommitBatchMaxWait:           Duration(time.Hour),
		PreCommitCheckInterval:          Duration(time.Minute),
		PreCommitGasOverEstimation:      0,
		ProCommitGasOverEstimation:      0,
		BatchPreCommitGasOverEstimation: 0,
		BatchProCommitGasOverEstimation: 0,
		MsgConfidence:                   10,
	}

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

func ExampleCommitmentMinerConfig() CommitmentMinerConfig {
	defaultCfg := DefaultCommitmentPolicy()
	return CommitmentMinerConfig{
		Controls: CommitmentControlAddress{
			PreCommit:   fakeAddress,
			ProveCommit: fakeAddress,
		},
		CommitmentPolicyConfigOptional: CommitmentPolicyConfigOptional{
			CommitCheckInterval:             &defaultCfg.CommitCheckInterval,
			PreCommitCheckInterval:          &defaultCfg.PreCommitCheckInterval,
			PreCommitGasOverEstimation:      &defaultCfg.PreCommitGasOverEstimation,
			ProCommitGasOverEstimation:      &defaultCfg.ProCommitGasOverEstimation,
			BatchPreCommitGasOverEstimation: &defaultCfg.BatchPreCommitGasOverEstimation,
			BatchProCommitGasOverEstimation: &defaultCfg.BatchProCommitGasOverEstimation,
			MsgConfidence:                   &defaultCfg.MsgConfidence,
		},
	}
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

type RegisterProofActorConfig struct {
	Apis  []string
	Token string
}

type RegisterProofConfig struct {
	Actors map[string]RegisterProofActorConfig
}

func DefaultRegisterProofConfig() RegisterProofConfig {
	return RegisterProofConfig{
		Actors: map[string]RegisterProofActorConfig{},
	}
}

type DealManagerConfig struct {
	Enable           bool
	MarketAPI        RPCClientConfig
	LocalPieceStores []filestore.Config
}

func DefaultDealManagerConfig() DealManagerConfig {
	return DealManagerConfig{
		Enable:           false,
		MarketAPI:        RPCClientConfig{},
		LocalPieceStores: []filestore.Config{},
	}
}

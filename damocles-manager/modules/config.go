package modules

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/messager"
	"github.com/samber/lo"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/confmgr"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore"
)

var log = logging.New("config")

func init() {
	fake, err := address.NewFromString("f1abjxfbp274xpdqcpuaykwkfb43omjotacm2p3za")
	if err != nil {
		panic(fmt.Errorf("parse fake address: %w", err))
	}

	fakeAddress = MustAddress(fake)
}

type SafeConfig struct {
	*Config
	sync.Locker
}

func (sc *SafeConfig) MustCommonConfig() CommonConfig {
	sc.Lock()
	defer sc.Unlock()

	return sc.Common
}

func (sc *SafeConfig) MustMinerConfig(mid abi.ActorID) MinerConfig {
	mc, err := sc.MinerConfig(mid)
	if err != nil {
		panic(err)
	}

	return mc
}

func (sc *SafeConfig) MinerConfig(mid abi.ActorID) (MinerConfig, error) {
	sc.Lock()
	defer sc.Unlock()

	for i := range sc.Miners {
		if sc.Miners[i].Actor == mid {
			return sc.Miners[i], nil
		}
	}

	return MinerConfig{}, fmt.Errorf("config for miner actor %d not found", mid)
}

var fakeAddress MustAddress

func GetFakeAddress() MustAddress {
	return fakeAddress
}

const ConfigKey = "sector-manager"

type CommonAPIConfig struct {
	Chain              *string
	Messager           *string
	Market             *string
	Gateway            []string
	Token              string
	ChainEventInterval Duration
}

func defaultCommonAPIConfig(example bool) CommonAPIConfig {
	cfg := CommonAPIConfig{
		ChainEventInterval: Duration(time.Minute),
	}
	if example {
		// cfg.Chain = "/ip4/{api_host}/tcp/{api_port}"
		// cfg.Messager = "/ip4/{api_host}/tcp/{api_port}"
		// cfg.Market = "/ip4/{api_host}/tcp/{api_port}"
		cfg.Gateway = []string{"/ip4/{api_host}/tcp/{api_port}"}
		cfg.Token = "{some token}"
	}
	return cfg
}

type PluginConfig struct {
	Dir string
}

func DefaultPluginConfig() *PluginConfig {
	return &PluginConfig{
		Dir: "",
	}
}

type DBConfig struct {
	Driver string
	Badger *KVStoreBadgerDBConfig
	Mongo  *KVStoreMongoDBConfig
	Plugin *KVStorePluginDBConfig
}

func DefaultDBConfig() *DBConfig {
	return &DBConfig{
		Driver: "badger",
		Badger: defaultBadgerDBConfig(),
		Mongo:  nil,
		Plugin: nil,
	}
}

type KVStoreBadgerDBConfig struct {
	BaseDir string
}

func defaultBadgerDBConfig() *KVStoreBadgerDBConfig {
	return &KVStoreBadgerDBConfig{
		BaseDir: "",
	}
}

type KVStoreMongoDBConfig struct {
	Enable       bool // For compatibility with v0.5
	DSN          string
	DatabaseName string
}

type KVStorePluginDBConfig struct {
	PluginName string
	Meta       map[string]string
}

type PieceStoreConfig struct {
	Name       string
	Path       string
	Meta       map[string]string
	ReadOnly   bool
	Plugin     string // For compatibility with v0.5
	PluginName string
}

type PersistStoreConfig struct {
	objstore.Config
	objstore.StoreSelectPolicy

	Plugin     string // For compatibility with v0.5
	PluginName string
}

type ProvingConfig struct {
	// Maximum number of sector checks to run in parallel. (0 = unlimited)
	//
	// WARNING: Setting this value too high may make the node crash by running out of stack
	// WARNING: Setting this value too low may make sector challenge reading much slower, resulting in failed PoSt due
	// to late submission.
	ParallelCheckLimit int

	// Maximum amount of time a proving pre-check can take for a sector.
	// If the check times out the sector will be skipped
	//
	// WARNING: Setting this value too low risks in sectors being skipped even though they are accessible,
	// just reading the test challenge took longer than this timeout
	// WARNING: Setting this value too high risks missing PoSt deadline in case IO operations
	// related to this sector are blocked (e.g. in case of disconnected NFS mount)
	SingleCheckTimeout Duration

	// Maximum amount of time a proving pre-check can take for an entire partition.
	// If the check times out, sectors in the partition which didn't get checked on time will be skipped
	//
	// WARNING: Setting this value too low risks in sectors being skipped even though they are accessible,
	// just reading the test challenge took longer than this timeout
	// WARNING: Setting this value too high risks missing PoSt deadline in case IO operations
	// related to this partition are blocked or slow
	PartitionCheckTimeout Duration

	WorkerProver *WorkerProverConfig
}

func defaultProvingConfig() ProvingConfig {
	cfg := ProvingConfig{
		ParallelCheckLimit:    128,
		PartitionCheckTimeout: Duration(20 * time.Minute),
		SingleCheckTimeout:    Duration(10 * time.Minute),
		WorkerProver:          DefaultWorkerProverConfig(),
	}
	return cfg
}

type WorkerProverConfig struct {
	// The maximum number of attempts of the WindowPoSt job,
	// job that exceeds the JobMaxTry number can only be re-executed by manual reset
	JobMaxTry uint32
	// The timeout of the WindowPoSt job's heartbeat
	// jobs that have not sent a heartbeat for more than this time will be set to fail and retried
	HeartbeatTimeout time.Duration
	// WindowPoSt jobs created longer than this time will be deleted
	JobLifetime time.Duration
}

func DefaultWorkerProverConfig() *WorkerProverConfig {
	return &WorkerProverConfig{
		JobMaxTry:        2,
		HeartbeatTimeout: 15 * time.Second,
		JobLifetime:      25 * time.Hour,
	}
}

type CommonConfig struct {
	API         CommonAPIConfig
	Plugins     *PluginConfig
	PieceStores []PieceStoreConfig

	// PersistStores should not be used directly, use GetPersistStores instead
	PersistStores []PersistStoreConfig
	// Configure the storage directory for scanning the directory that contains `sectorstore.json` file,
	// supporting `glob` mode.
	ScanPersistStores []string

	MongoKVStore *KVStoreMongoDBConfig // For compatibility with v0.5
	DB           *DBConfig
	Proving      ProvingConfig
}

func (c CommonConfig) GetPersistStores() (cfgs []PersistStoreConfig, err error) {
	cfgs = make([]PersistStoreConfig, len(c.PersistStores))
	copy(cfgs, c.PersistStores)

	if len(c.ScanPersistStores) != 0 {
		var scanned []PersistStoreConfig
		scanned, err = ScanPersistStores(c.ScanPersistStores)
		if err != nil {
			return
		}
		cfgs = append(cfgs, scanned...) //nolint:makezero
	}

	checkName := make(map[string]struct{})
	for i := range cfgs {
		if cfgs[i].Name == "" {
			cfgs[i].Name = cfgs[i].Path
		}
		if _, ok := checkName[cfgs[i].Name]; ok {
			return nil, fmt.Errorf("duplicate persist store name %s", cfgs[i].Name)
		}
		checkName[cfgs[i].Name] = struct{}{}
	}

	return
}

// [path]/sectorstore.json
type SectorStoreJSON struct {
	// `ID` is the name of the storage
	// Equivalent to `Name`
	// `ID` is for compatibility with lotus
	ID string

	PersistStoreConfig
}

func exampleFilestoreConfig() objstore.Config {
	cfg := objstore.DefaultConfig("{store_path}", false)
	cfg.Name = "{store_name}"
	cfg.Meta["SomeKey"] = "SomeValue"
	return cfg
}

func defaultCommonConfig(example bool) CommonConfig {
	cfg := CommonConfig{
		API:               defaultCommonAPIConfig(example),
		PieceStores:       []PieceStoreConfig{},
		PersistStores:     []PersistStoreConfig{},
		ScanPersistStores: []string{},
		MongoKVStore:      nil,
		DB:                DefaultDBConfig(),
		Proving:           defaultProvingConfig(),
	}

	if example {
		exampleCfg := exampleFilestoreConfig()
		cfg.PieceStores = append(cfg.PieceStores, PieceStoreConfig{
			Name:       exampleCfg.Name,
			Path:       exampleCfg.Path,
			Meta:       exampleCfg.Meta,
			PluginName: "s3store",
		})

		cfg.PersistStores = append(cfg.PersistStores, PersistStoreConfig{
			Config: objstore.Config{
				Name: exampleCfg.Name,
				Path: exampleCfg.Path,
				Meta: exampleCfg.Meta,
			},
			StoreSelectPolicy: objstore.StoreSelectPolicy{
				AllowMiners: []abi.ActorID{1, 2},
				DenyMiners:  []abi.ActorID{3, 4},
			},
			PluginName: "s3store",
		})
	}

	return cfg
}

type FeeConfig struct {
	// included in msg send spec
	GasOverEstimation float64
	GasOverPremium    float64
	// set to msg directly
	GasFeeCap FIL
	MaxFeeCap FIL // 兼容老字段， FeeConfig用在embed字段， 使用TextUnmarshaler会影响上层结构体解析
}

func (feeCfg *FeeConfig) GetGasFeeCap() FIL {
	gasFeeCap := feeCfg.GasFeeCap
	if (*big.Int)(&feeCfg.GasFeeCap).NilOrZero() && !(*big.Int)(&feeCfg.MaxFeeCap).NilOrZero() {
		// if not set GasFeeCap but set MaxFeecap use MaxFeecap as GasFeecap
		gasFeeCap = feeCfg.MaxFeeCap
	}
	return gasFeeCap
}

func (feeCfg *FeeConfig) GetSendSpec() messager.MsgMeta {
	return messager.MsgMeta{
		GasOverEstimation: feeCfg.GasOverEstimation,
		GasOverPremium:    feeCfg.GasOverPremium,
	}
}

func defaultFeeConfig() FeeConfig {
	return FeeConfig{
		GasOverEstimation: 1.2,
		GasOverPremium:    0,
		GasFeeCap:         NanoFIL.Mul(5),
	}
}

type MinerSectorConfig struct {
	InitNumber   uint64
	MinNumber    *uint64
	MaxNumber    *uint64
	Enabled      bool
	EnableDeals  bool
	LifetimeDays uint64
	Verbose      bool
}

func defaultMinerSectorConfig(example bool) MinerSectorConfig {
	cfg := MinerSectorConfig{
		InitNumber:   0,
		Enabled:      true,
		LifetimeDays: 540,
		Verbose:      false,
	}

	if example {
		min := uint64(10)
		max := uint64(1_000_000)
		cfg.MinNumber = &min
		cfg.MaxNumber = &max
	}
	return cfg
}

type MinerSnapUpRetryConfig struct {
	MaxAttempts      *int
	PollInterval     Duration
	APIFailureWait   Duration
	LocalFailureWait Duration
}

func defaultMinerSnapUpRetryConfig(example bool) MinerSnapUpRetryConfig {
	cfg := MinerSnapUpRetryConfig{
		MaxAttempts:      nil,
		PollInterval:     Duration(3 * time.Minute),
		APIFailureWait:   Duration(3 * time.Minute),
		LocalFailureWait: Duration(3 * time.Minute),
	}

	if example {
		maxAttempts := 10
		cfg.MaxAttempts = &maxAttempts
	}

	return cfg
}

type MinerSnapUpConfig struct {
	Enabled bool
	Sender  *MustAddress
	Senders []MustAddress // allows multiple senders to be specified

	SendFund bool
	FeeConfig
	CleanupCCData bool

	MessageConfidential *abi.ChainEpoch
	ReleaseCondidential *abi.ChainEpoch

	MessageConfidence abi.ChainEpoch
	ReleaseConfidence abi.ChainEpoch

	Retry MinerSnapUpRetryConfig
}

func (m *MinerSnapUpConfig) GetSenders() []address.Address {
	stdSenders := lo.Map(m.Senders, func(item MustAddress, _ int) address.Address { return item.Std() })
	if m.Sender != nil {
		return lo.Union(stdSenders, []address.Address{m.Sender.Std()})
	}
	return lo.Uniq(stdSenders)
}

func (m *MinerSnapUpConfig) GetMessageConfidence() abi.ChainEpoch {
	if m.MessageConfidential != nil {
		return *m.MessageConfidential
	}

	return m.MessageConfidence
}

func (m *MinerSnapUpConfig) GetReleaseConfidence() abi.ChainEpoch {
	if m.ReleaseCondidential != nil {
		return *m.ReleaseCondidential
	}

	return m.ReleaseConfidence
}

func defaultMinerSnapUpConfig(example bool) MinerSnapUpConfig {
	cfg := MinerSnapUpConfig{
		Enabled:           false,
		SendFund:          true,
		FeeConfig:         defaultFeeConfig(),
		CleanupCCData:     true,
		MessageConfidence: 15,
		ReleaseConfidence: 30,
		Retry:             defaultMinerSnapUpRetryConfig(example),
	}

	if example {
		cfg.Senders = []MustAddress{fakeAddress}
	}

	return cfg
}

type MinerCommitmentConfig struct {
	Confidence int64
	Pre        MinerCommitmentPolicyConfig
	Prove      MinerCommitmentPolicyConfig
	Terminate  MinerCommitmentPolicyConfig
}

func defaultMinerCommitmentConfig(example bool) MinerCommitmentConfig {
	cfg := MinerCommitmentConfig{
		Confidence: 10,
		Pre:        defaultMinerCommitmentPolicyConfig(example),
		Prove:      defaultMinerCommitmentPolicyConfig(example),
		Terminate:  defaultMinerCommitmentPolicyConfig(example),
	}

	cfg.Terminate.Batch.Threshold = 2
	cfg.Pre.Batch.Threshold = 2
	cfg.Prove.Batch.Threshold = 4

	return cfg
}

type MinerCommitmentPolicyConfig struct {
	Sender  *MustAddress
	Senders []MustAddress // allows multiple senders to be specified

	SendFund bool
	FeeConfig
	Batch MinerCommitmentBatchPolicyConfig
}

func (m *MinerCommitmentPolicyConfig) GetSenders() []address.Address {
	stdSenders := lo.Map(m.Senders, func(item MustAddress, _ int) address.Address { return item.Std() })
	if m.Sender != nil {
		return lo.Union(stdSenders, []address.Address{m.Sender.Std()})
	}
	return lo.Uniq(stdSenders)
}

func defaultMinerCommitmentPolicyConfig(example bool) MinerCommitmentPolicyConfig {
	cfg := MinerCommitmentPolicyConfig{
		SendFund:  true,
		FeeConfig: defaultFeeConfig(),
		Batch:     defaultMinerCommitmentBatchPolicyConfig(),
	}

	if example {
		cfg.Senders = []MustAddress{fakeAddress}
	}

	return cfg
}

type MinerCommitmentBatchPolicyConfig struct {
	Threshold     int
	MaxWait       Duration
	CheckInterval Duration
	FeeConfig
}

func defaultMinerCommitmentBatchPolicyConfig() MinerCommitmentBatchPolicyConfig {
	cfg := MinerCommitmentBatchPolicyConfig{
		Threshold:     4,
		MaxWait:       Duration(time.Hour),
		CheckInterval: Duration(time.Minute),
		FeeConfig:     defaultFeeConfig(),
	}

	return cfg
}

type MinerPoStConfig struct {
	Sender  *MustAddress
	Senders []MustAddress // allows multiple senders to be specified

	Enabled     bool
	StrictCheck bool
	Parallel    bool
	FeeConfig
	Confidence                      uint64
	SubmitConfidence                uint64
	ChallengeConfidence             uint64
	MaxRecoverSectorLimit           uint64
	MaxPartitionsPerPoStMessage     uint64
	MaxPartitionsPerRecoveryMessage uint64
}

func (m *MinerPoStConfig) GetSenders() []address.Address {
	stdSenders := lo.Map(m.Senders, func(item MustAddress, _ int) address.Address { return item.Std() })
	if m.Sender != nil {
		return lo.Union(stdSenders, []address.Address{m.Sender.Std()})
	}
	return lo.Uniq(stdSenders)
}

func DefaultMinerPoStConfig(example bool) MinerPoStConfig {
	cfg := MinerPoStConfig{
		Enabled:                         true,
		StrictCheck:                     true,
		Parallel:                        false,
		FeeConfig:                       defaultFeeConfig(),
		Confidence:                      10,
		SubmitConfidence:                0,
		ChallengeConfidence:             0,
		MaxRecoverSectorLimit:           0,
		MaxPartitionsPerPoStMessage:     0,
		MaxPartitionsPerRecoveryMessage: 0,
	}

	if example {
		cfg.Senders = []MustAddress{fakeAddress}
	}

	return cfg
}

type MinerProofConfig struct {
	Enabled bool
}

func defaultMinerProofConfig() MinerProofConfig {
	return MinerProofConfig{
		Enabled: false,
	}
}

type MinerSealingConfig struct {
	SealingEpochDuration int64

	RequireActivationSuccess         bool
	RequireActivationSuccessUpdate   bool
	RequireNotificationSuccess       bool
	RequireNotificationSuccessUpdate bool

	UseSyntheticPoRep bool

	SealingSectorDeadlineDelayNi uint64
}

func defaultMinerSealingConfig() MinerSealingConfig {
	return MinerSealingConfig{
		SealingEpochDuration: 0,

		// Whether to abort if any sector activation in a batch fails
		// (newly sealed sectors, only with ProveCommitSectors3).
		RequireActivationSuccess: false,
		// Whether to abort if any piece activation notification returns a non-zero exit code
		// (newly sealed sectors, only with ProveCommitSectors3).
		RequireActivationSuccessUpdate: false,
		// Whether to abort if any sector activation in a batch fails
		// (updating sectors, only with ProveReplicaUpdates3).
		RequireNotificationSuccess: false,
		// Whether to abort if any piece activation notification returns a non-zero exit code
		// (updating sectors, only with ProveReplicaUpdates3).
		RequireNotificationSuccessUpdate: false,

		UseSyntheticPoRep: false,

		SealingSectorDeadlineDelayNi: 6,
	}
}

type MinerConfig struct {
	Actor      abi.ActorID
	Sector     MinerSectorConfig
	SnapUp     MinerSnapUpConfig
	Commitment MinerCommitmentConfig
	PoSt       MinerPoStConfig
	Proof      MinerProofConfig
	Sealing    MinerSealingConfig
}

func DefaultMinerConfig(example bool) MinerConfig {
	cfg := MinerConfig{
		Sector:     defaultMinerSectorConfig(example),
		SnapUp:     defaultMinerSnapUpConfig(example),
		Commitment: defaultMinerCommitmentConfig(example),
		PoSt:       DefaultMinerPoStConfig(example),
		Proof:      defaultMinerProofConfig(),
		Sealing:    defaultMinerSealingConfig(),
	}

	if example {
		cfg.Actor = 10086
	}

	return cfg
}

var _ confmgr.ConfigUnmarshaller = (*Config)(nil)

type Config struct {
	Common CommonConfig
	Miners []MinerConfig
}

func (c *Config) UnmarshalConfig(data []byte) error {
	primitive := struct {
		Common CommonConfig
		Miners []toml.Primitive
	}{
		Common: defaultCommonConfig(false),
	}

	meta, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&primitive)
	if err != nil {
		return fmt.Errorf("toml.Unmarshal to primitive: %w", err)
	}

	miners := make([]MinerConfig, 0, len(primitive.Miners))
	for i, pm := range primitive.Miners {
		mcfg := DefaultMinerConfig(false)
		err := meta.PrimitiveDecode(pm, &mcfg)
		if err != nil {
			return fmt.Errorf("decode primitive to miner config #%d: %w", i, err)
		}

		miners = append(miners, mcfg)
	}

	c.Common = primitive.Common
	c.Miners = miners
	return nil
}

func DefaultConfig(example bool) Config {
	cfg := Config{
		Common: defaultCommonConfig(example),
	}

	if example {
		cfg.Miners = append(cfg.Miners, DefaultMinerConfig(example))
	}

	return cfg
}

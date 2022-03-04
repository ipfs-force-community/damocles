package modules

import (
	"fmt"
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

type SafeConfig struct {
	*Config
	sync.Locker
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

const ConfigKey = "sector-manager"

type CommonAPIConfig struct {
	Chain    string
	Messager string
	Market   string
	Gateway  []string
	Token    string
}

func defaultCommonAPIConfig(example bool) CommonAPIConfig {
	cfg := CommonAPIConfig{}
	if example {
		cfg.Chain = "/ip4/{api_host}/tcp/{api_port}"
		cfg.Messager = "/ip4/{api_host}/tcp/{api_port}"
		cfg.Market = "/ip4/{api_host}/tcp/{api_port}"
		cfg.Gateway = []string{"/ip4/{api_host}/tcp/{api_port}"}
		cfg.Token = "{some token}"
	}
	return cfg
}

type CommonConfig struct {
	API           CommonAPIConfig
	PieceStores   []filestore.Config
	PersistStores []filestore.Config
}

func exampleFilestoreConfig() filestore.Config {
	return filestore.Config{
		Name: "{store_name}",
		Path: "{store_path}",
	}
}

func defaultCommonConfig(example bool) CommonConfig {
	cfg := CommonConfig{
		API:           defaultCommonAPIConfig(example),
		PieceStores:   []filestore.Config{},
		PersistStores: []filestore.Config{},
	}

	if example {
		cfg.PieceStores = append(cfg.PieceStores, exampleFilestoreConfig())
		cfg.PersistStores = append(cfg.PersistStores, exampleFilestoreConfig())
	}

	return cfg
}

type FeeConfig struct {
	GasOverEstimation float64
	MaxFeeCap         FIL
}

func defaultFeeConfig() FeeConfig {
	return FeeConfig{
		GasOverEstimation: 1.2,
		MaxFeeCap:         NanoFIL.Mul(5),
	}
}

type MinerSectorConfig struct {
	InitNumber uint64
	MaxNumber  *uint64
	Enabled    bool
}

func defaultMinerSectorConfig(example bool) MinerSectorConfig {
	cfg := MinerSectorConfig{
		InitNumber: 0,
		Enabled:    true,
	}

	if example {
		max := uint64(1_000_000)
		cfg.MaxNumber = &max
	}

	return cfg
}

type MinerCommitmentConfig struct {
	Pre   MinerCommitmentPolicyConfig
	Prove MinerCommitmentPolicyConfig
}

func defaultMinerCommitmentConfig(example bool) MinerCommitmentConfig {
	cfg := MinerCommitmentConfig{
		Pre:   defaultMinerCommitmentPolicyConfig(example),
		Prove: defaultMinerCommitmentPolicyConfig(example),
	}

	return cfg
}

type MinerCommitmentPolicyConfig struct {
	Sender MustAddress
	FeeConfig
	Batch MinerCommitmentBatchPolicyConfig
}

func defaultMinerCommitmentPolicyConfig(example bool) MinerCommitmentPolicyConfig {
	cfg := MinerCommitmentPolicyConfig{
		FeeConfig: defaultFeeConfig(),
		Batch:     defaultMinerCommitmentBatchPolicyConfig(),
	}

	if example {
		cfg.Sender = fakeAddress
	}

	return cfg
}

type MinerCommitmentBatchPolicyConfig struct {
	Enabled       bool
	Threshold     int
	MaxWait       Duration
	CheckInterval Duration
	FeeConfig
}

func defaultMinerCommitmentBatchPolicyConfig() MinerCommitmentBatchPolicyConfig {
	cfg := MinerCommitmentBatchPolicyConfig{
		Enabled:       false,
		Threshold:     16,
		MaxWait:       Duration(time.Hour),
		CheckInterval: Duration(time.Minute),
		FeeConfig:     defaultFeeConfig(),
	}

	return cfg
}

type MinerPoStConfig struct {
	Sender      MustAddress
	Enabled     bool
	StrictCheck bool
	FeeConfig
	Confidence uint64
}

func defaultMinerPoStConfig(example bool) MinerPoStConfig {
	cfg := MinerPoStConfig{
		Enabled:     true,
		StrictCheck: true,
		FeeConfig:   defaultFeeConfig(),
		Confidence:  10,
	}

	if example {
		cfg.Sender = fakeAddress
	}

	return cfg
}

type MinerDealConfig struct {
	Enabled bool
}

func defaultMinerDealConfig() MinerDealConfig {
	return MinerDealConfig{
		Enabled: false,
	}
}

type MinerProofConfig struct {
	Enabled bool
}

func defaultMinerProofConfig() MinerProofConfig {
	return MinerProofConfig{
		Enabled: false,
	}
}

type MinerConfig struct {
	Actor      abi.ActorID
	Sector     MinerSectorConfig
	Commitment MinerCommitmentConfig
	PoSt       MinerPoStConfig
	Proof      MinerProofConfig
	Deal       MinerDealConfig
}

func defaultMinerConfig(example bool) MinerConfig {
	cfg := MinerConfig{
		Sector:     defaultMinerSectorConfig(example),
		Commitment: defaultMinerCommitmentConfig(example),
		PoSt:       defaultMinerPoStConfig(example),
		Proof:      defaultMinerProofConfig(),
		Deal:       defaultMinerDealConfig(),
	}

	if example {
		cfg.Actor = 10086
	}

	return cfg
}

type Config struct {
	Common CommonConfig
	Miners []MinerConfig
}

func DefaultConfig(example bool) Config {
	cfg := Config{
		Common: defaultCommonConfig(example),
	}

	if example {
		cfg.Miners = append(cfg.Miners, defaultMinerConfig(example))
	}

	return cfg
}

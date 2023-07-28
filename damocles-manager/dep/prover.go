package dep

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover/ext"
	proverworker "github.com/ipfs-force-community/damocles/damocles-manager/modules/impl/prover/worker"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/confmgr"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

type (
	WorkerProverStore kvstore.KVStore
)

func ExtProver() dix.Option {
	return dix.Options(
		dix.Override(new(*modules.ProcessorConfig), ProvideExtProverConfig),
		dix.Override(new(*ext.Prover), BuildExtProver),
		dix.Override(new(core.Prover), dix.From(new(*ext.Prover))),
	)
}

func WorkerProver() dix.Option {
	return dix.Options(
		dix.Override(new(WorkerProverStore), BuildWorkerProverStore),
		dix.Override(new(core.WorkerWdPoStJobManager), BuildWorkerWdPoStJobManager),
		dix.Override(new(core.WorkerWdPoStAPI), proverworker.NewWdPoStAPIImpl),
		dix.Override(new(core.Prover), BuildWorkerProver),
	)
}

func BuildExtProver(gctx GlobalContext, lc fx.Lifecycle, sectorTracker core.SectorTracker, cfg *modules.ProcessorConfig) (*ext.Prover, error) {
	p, err := ext.New(gctx, sectorTracker, cfg.WdPost, cfg.WinPost)
	if err != nil {
		return nil, fmt.Errorf("construct ext prover: %w", err)
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			p.Run()
			return nil
		},

		OnStop: func(context.Context) error {
			p.Close()
			return nil
		},
	})

	return p, nil
}

func ProvideExtProverConfig(gctx GlobalContext, lc fx.Lifecycle, cfgmgr confmgr.ConfigManager, locker confmgr.WLocker) (*modules.ProcessorConfig, error) {
	cfg := modules.DefaultProcessorConfig(false)
	if err := cfgmgr.Load(gctx, modules.ProcessorConfigKey, &cfg); err != nil {
		return nil, err
	}

	buf := bytes.Buffer{}
	encode := toml.NewEncoder(&buf)
	encode.Indent = ""
	err := encode.Encode(cfg)
	if err != nil {
		return nil, err
	}

	log.Infof("ext porver initial cfg: \n%s\n", buf.String())

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Watch(gctx, modules.ProcessorConfigKey, &cfg, locker, func() interface{} {
				c := modules.DefaultProcessorConfig(false)
				return &c
			})
		},
	})

	return &cfg, nil
}

func BuildWorkerProverStore(gctx GlobalContext, db UnderlyingDB) (WorkerProverStore, error) {
	return db.OpenCollection(gctx, "prover")
}

func BuildWorkerProver(lc fx.Lifecycle, jobMgr core.WorkerWdPoStJobManager, sectorTracker core.SectorTracker, scfg *modules.SafeConfig) (core.Prover, error) {
	cfg := scfg.MustCommonConfig()
	p := proverworker.NewProver(jobMgr, sectorTracker, &proverworker.Config{
		RetryFailedJobsInterval:    10 * time.Second,
		JobMaxTry:                  cfg.Proving.WorkerProver.JobMaxTry,
		HeartbeatTimeout:           cfg.Proving.WorkerProver.HeartbeatTimeout,
		CleanupExpiredJobsInterval: 30 * time.Minute,
		JobLifetime:                cfg.Proving.WorkerProver.JobLifetime,
	})
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			p.Start(ctx)
			return nil
		},
	})

	return p, nil
}

func BuildWorkerWdPoStJobManager(kv WorkerProverStore) (core.WorkerWdPoStJobManager, error) {
	wdpostKV, err := kvstore.NewWrappedKVStore([]byte("wdpost-"), kv)
	if err != nil {
		return nil, err
	}
	return proverworker.NewKVJobManager(*kvstore.NewKVExt(wdpostKV)), nil
}

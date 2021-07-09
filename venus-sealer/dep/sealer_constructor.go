package dep

import (
	"context"

	"go.uber.org/fx"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/homedir"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
	"github.com/dtynn/venus-cluster/venus-sealer/sealer/impl/sectors"
)

func BuildLocalSectorManager(cfg *sealer.Config, locker confmgr.RLocker, mapi api.MinerInfoAPI, numAlloc api.SectorNumberAllocator) (api.SectorManager, error) {
	return sectors.NewManager(cfg, locker, mapi, numAlloc)
}

func BuildLocalConfigManager(gctx GlobalContext, lc fx.Lifecycle, home *homedir.Home) (confmgr.ConfigManager, error) {
	cfgmgr, err := confmgr.NewLocal(home.Dir())
	if err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Run(gctx)
		},
		OnStop: func(ctx context.Context) error {
			return cfgmgr.Close(ctx)
		},
	})

	return cfgmgr, nil
}

func ProvideSealerConfig(gctx GlobalContext, lc fx.Lifecycle, cfgmgr confmgr.ConfigManager, locker confmgr.WLocker) (*sealer.Config, error) {
	cfg := sealer.DefaultConfig()
	if err := cfgmgr.Load(gctx, sealer.ConfigKey, &cfg); err != nil {
		return nil, err
	}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			return cfgmgr.Watch(gctx, sealer.ConfigKey, &cfg, locker, func() interface{} {
				c := sealer.DefaultConfig()
				return &c
			})
		},
	})

	return &cfg, nil
}

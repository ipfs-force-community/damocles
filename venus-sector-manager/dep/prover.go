package dep

import (
	"bytes"
	"context"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/dtynn/dix"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover/ext"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
)

func ExtProver() dix.Option {
	return dix.Options(
		dix.Override(new(*ext.Config), ProvideExtProverConfig),
		dix.Override(new(*ext.Prover), BuildExtProver),
		dix.Override(new(core.Prover), dix.From(new(*ext.Prover))),
	)
}

func BuildExtProver(gctx GlobalContext, lc fx.Lifecycle, cfg *ext.Config) (*ext.Prover, error) {
	p, err := ext.New(gctx, cfg.WdPost)
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

func ProvideExtProverConfig(gctx GlobalContext, lc fx.Lifecycle, cfgmgr confmgr.ConfigManager, locker confmgr.WLocker) (*ext.Config, error) {
	cfg := ext.DefaultConfig(false)
	if err := cfgmgr.Load(gctx, ext.ConfigKey, &cfg); err != nil {
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
			return cfgmgr.Watch(gctx, ext.ConfigKey, &cfg, locker, func() interface{} {
				c := ext.DefaultConfig(false)
				return &c
			})
		},
	})

	return &cfg, nil
}

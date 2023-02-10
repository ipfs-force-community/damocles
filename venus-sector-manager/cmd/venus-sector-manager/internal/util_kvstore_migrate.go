package internal

import (
	"context"
	"fmt"
	"sync"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/dep"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/confmgr"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/homedir"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	vsmplugin "github.com/ipfs-force-community/venus-cluster/vsm-plugin"
)

var defaultSubStore = []string{"common", "meta", "offline_meta", "sector-index", "snapup", "worker"}

var utilMigrate = &cli.Command{
	Name:  "migrate",
	Usage: "Migrating venus-sector-manager's database data",
	Flags: []cli.Flag{
		HomeFlag,
		&cli.StringFlag{
			Name:        "from",
			Usage:       "the source database",
			DefaultText: "the database currently in use. (read [Common.DB.Driver] value in the config file)",
		},
		&cli.StringFlag{
			Name:     "to",
			Usage:    "the dest database",
			Required: true,
		},
		&cli.StringSliceFlag{
			Name:  "sub-stores",
			Usage: "sub stores name which need migrate",
			Value: cli.NewStringSlice(defaultSubStore...),
		},
	},
	Action: func(cctx *cli.Context) error {
		from := cctx.String("from")
		to := cctx.String("to")
		subStores := cctx.StringSlice("sub-stores")

		gctx := new(dep.GlobalContext)
		lc := new(fx.Lifecycle)
		scfg := new(*modules.SafeConfig)
		home := new(*homedir.Home)
		loadedPlugins := new(*vsmplugin.LoadedPlugins)

		stop, err := Dep(cctx, gctx, lc, scfg, home, loadedPlugins)
		if err != nil {
			return fmt.Errorf("construct dep: %w", err)
		}
		defer stop()

		commonCfg := (*scfg).MustCommonConfig()
		if commonCfg.DB == nil {
			commonCfg.DB = modules.DefaultDBConfig()
		}

		dbCfg := *commonCfg.DB
		if from == "" {
			from = dbCfg.Driver
		}

		dbCfg.Driver = from
		fromDB, err := dep.BuildKVStoreDB(*gctx, *lc, dbCfg, *home, *loadedPlugins)
		if err != nil {
			return fmt.Errorf("failed to build from db: '%s', %w", dbCfg.Driver, err)
		}
		dbCfg.Driver = to
		toDB, err := dep.BuildKVStoreDB(*gctx, *lc, dbCfg, *home, *loadedPlugins)
		if err != nil {
			return fmt.Errorf("failed to build to db: '%s', %w", dbCfg.Driver, err)
		}
		for _, sub := range subStores {
			fromKV, err := fromDB.OpenCollection(cctx.Context, sub)
			if err != nil {
				return fmt.Errorf("open from db collection: '%s/%s', %w", from, sub, err)
			}
			toKV, err := toDB.OpenCollection(cctx.Context, sub)
			if err != nil {
				return fmt.Errorf("open to db collection: '%s/%s', %w", to, sub, err)
			}

			err = migrate(cctx.Context, fromKV, toKV)
			if err != nil {
				return err
			}
			fmt.Printf("'%s' migrated\n", sub)
		}
		return nil
	},
}

func migrate(ctx context.Context, src, dst kvstore.KVStore) error {
	iter, err := src.Scan(ctx, nil)
	if err != nil {
		return err
	}

	for iter.Next() {
		v := kvstore.Val{}
		err = iter.View(ctx, func(val kvstore.Val) error {
			v = val
			return nil
		})
		if err != nil {
			return err
		}

		err = dst.Put(ctx, iter.Key(), v)
		if err != nil {
			return err
		}
	}
	return nil
}

func Dep(cctx *cli.Context, wants ...interface{}) (stop func(), err error) {
	gctx, gcancel := NewSigContext(cctx.Context)
	cfgmu := &sync.RWMutex{}

	var stopper dix.StopFunc
	stopper, err = dix.New(
		gctx,
		dix.Options(
			dix.Override(new(confmgr.WLocker), cfgmu),
			dix.Override(new(confmgr.RLocker), cfgmu.RLocker()),
			dix.Override(new(confmgr.ConfigManager), dep.BuildLocalConfigManager),
			dix.Override(new(dep.ConfDirPath), dep.BuildConfDirPath),
			dix.Override(new(*modules.Config), dep.ProvideConfig),
			dix.Override(new(*modules.SafeConfig), dep.ProvideSafeConfig),
			dix.Override(new(*vsmplugin.LoadedPlugins), dep.ProvidePlugins),
			dix.If(len(wants) > 0, dix.Populate(dep.InvokePopulate, wants...)),
		),
		DepsFromCLICtx(cctx),
		dix.Override(new(dep.GlobalContext), gctx),
	)

	if err != nil {
		gcancel()
		return
	}

	stop = func() {
		stopper(cctx.Context) // nolint: errcheck
		gcancel()
	}
	return
}

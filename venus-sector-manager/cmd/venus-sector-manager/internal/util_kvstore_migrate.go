package internal

import (
	"context"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var defaultSubStore = []string{"common", "meta", "offline_meta", "sector-index", "snapup", "worker"}

var utilMigrateBadgerMongo = &cli.Command{
	Name:  "migrate-badger-mongo",
	Usage: "migrate badger data into mongo default, can reverse with flag",
	Flags: []cli.Flag{
		HomeFlag,
		&cli.StringSliceFlag{
			Name:  "sub-stores",
			Usage: "sub stores name which need migrate",
			Value: cli.NewStringSlice(defaultSubStore...),
		},
		&cli.StringFlag{
			Name:     "mongo-dsn",
			Usage:    "the dsn of mongo kv",
			Required: true,
		},
		&cli.StringFlag{
			Name:     "mongo-dbname",
			Usage:    "the db name of mongo kv",
			Required: true,
		},
		&cli.BoolFlag{
			Name:  "reverse",
			Usage: "reverse dst and src kv-store, if this flag is true, will migrate data from mongo into local badger",
			Value: false,
		},
	},
	Action: func(cctx *cli.Context) error {
		home, err := HomeFromCLICtx(cctx)
		if err != nil {
			return err
		}
		reverse := cctx.Bool("reverse")
		subStores := cctx.StringSlice("sub-stores")
		for i := range subStores {
			badger, err := kvstore.OpenBadger(kvstore.DefaultBadgerOption(home.Sub(subStores[i])))
			if err != nil {
				return err
			}
			mongo, err := kvstore.OpenMongo(cctx.Context, cctx.String("mongo-dsn"), cctx.String("mongo-dbname"), subStores[i])
			if err != nil {
				return err
			}
			if reverse {
				err = migrate(cctx.Context, mongo, badger)
				if err != nil {
					return err
				}
				continue
			}
			err = migrate(cctx.Context, badger, mongo)
			if err != nil {
				return err
			}
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

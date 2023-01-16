package internal

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
)

var defaultSubStore = []string{"common", "meta", "offline_meta", "sector-index", "snapup", "worker"}

var utilMigrateBadgerMongo = &cli.Command{
	Name:  "migrate-badger-mongo",
	Usage: "migrate badger data into mongo default, can reverse with flag",
	Flags: []cli.Flag{
		HomeFlag,
		&cli.StringFlag{
			Name:        "badger-basedir",
			Usage:       "specify the basedir of badger databases",
			DefaultText: "the home dir",
		},
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
		badgerBasedir := cctx.String("badger-basedir")
		mongoDSN := cctx.String("mongo-dsn")

		if badgerBasedir == "" {
			badgerBasedir = home.Dir()
		}
		badger := kvstore.OpenBadger(badgerBasedir)
		mongo, err := kvstore.OpenMongo(cctx.Context, cctx.String("mongo-dsn"), cctx.String("mongo-dbname"))
		if err != nil {
			return fmt.Errorf("open mongodb dsn: %s, %w", mongoDSN, err)
		}

		for _, sub := range subStores {
			badgerKV, err := badger.OpenCollection(cctx.Context, sub)
			if err != nil {
				return fmt.Errorf("open badger collection: %s, %w", sub, err)
			}
			mongoKV, err := mongo.OpenCollection(cctx.Context, sub)
			if err != nil {
				return fmt.Errorf("open mongodb collection: %s, %w", sub, err)
			}

			src, dst := badgerKV, mongoKV
			if reverse {
				src, dst = mongoKV, badgerKV
			}

			err = migrate(cctx.Context, src, dst)
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

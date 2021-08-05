package internal

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/dep"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/homedir"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
)

var Log = logging.New("sealer")

var HomeFlag = &cli.StringFlag{
	Name:  "home",
	Value: "~/.venus-sealer",
}

type stopper = func()

func NewSigContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGABRT, syscall.SIGTERM, syscall.SIGINT)
}

func DepsFromCLICtx(cctx *cli.Context) dix.Option {
	return dix.Options(
		dix.Override(new(*cli.Context), cctx),
		dix.Override(new(*homedir.Home), HomeFromCLICtx),
	)
}

func HomeFromCLICtx(cctx *cli.Context) (*homedir.Home, error) {
	home, err := homedir.Open(cctx.String(HomeFlag.Name))
	if err != nil {
		return nil, fmt.Errorf("open home: %w", err)
	}

	if err := home.Init(); err != nil {
		return nil, fmt.Errorf("init home: %w", err)
	}

	return home, nil
}

func extractChainAPI(cctx *cli.Context) (chain.API, context.Context, stopper, error) {
	logging.SetupForSub("sealer")

	gctx, gcancel := NewSigContext(cctx.Context)

	var api chain.API

	stopper, err := dix.New(
		gctx,
		DepsFromCLICtx(cctx),
		dix.Override(new(dep.GlobalContext), gctx),
		dep.Chain(&api),
	)

	if err != nil {
		gcancel()
		return api, nil, nil, fmt.Errorf("construct sealer api: %w", err)
	}

	return api, gctx, func() {
		stopper(cctx.Context)
		gcancel()
	}, nil
}

func RPCCallError(method string, err error) error {
	return fmt.Errorf("rpc %s: %w", method, err)
}

package internal

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dtynn/dix"
	"github.com/urfave/cli/v2"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/homedir"
	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
)

var Log = logging.New("sealer")

var HomeFlag = &cli.StringFlag{
	Name:  "home",
	Value: "~/.venus-sealer",
}

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

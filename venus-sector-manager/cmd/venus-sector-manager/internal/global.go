package internal

import (
	"context"
	"fmt"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/dtynn/dix"
	"github.com/filecoin-project/go-address"
	"github.com/urfave/cli/v2"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/dep"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/homedir"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

var Log = logging.New("venus-sector-manager")

var HomeFlag = &cli.StringFlag{
	Name:  "home",
	Value: "~/.venus-sector-manager",
}

var NetFlag = &cli.StringFlag{
	Name:  "net",
	Value: "mainnet",
}

var SealerListenFlag = &cli.StringFlag{
	Name:  "listen",
	Value: ":1789",
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

type API struct {
	Chain    chain.API
	Messager messager.API
}

func extractAPI(cctx *cli.Context) (*API, context.Context, stopper, error) {
	logging.SetupForSub("sealer")

	gctx, gcancel := NewSigContext(cctx.Context)

	var capi chain.API
	var mapi messager.API

	stopper, err := dix.New(
		gctx,
		DepsFromCLICtx(cctx),
		dix.Override(new(dep.GlobalContext), gctx),
		dep.API(&capi, &mapi),
	)

	if err != nil {
		gcancel()
		return nil, nil, nil, fmt.Errorf("construct sealer api: %w", err)
	}

	return &API{
			Chain:    capi,
			Messager: mapi,
		}, gctx, func() {
			stopper(cctx.Context)
			gcancel()
		}, nil
}

func RPCCallError(method string, err error) error {
	return fmt.Errorf("rpc %s: %w", method, err)
}

var ErrEmptyAddressString = fmt.Errorf("empty address string")

func ShouldAddress(s string, checkEmpty bool, allowActor bool) (address.Address, error) {
	if checkEmpty && s == "" {
		return address.Undef, ErrEmptyAddressString
	}

	if allowActor {
		id, err := strconv.ParseUint(s, 10, 64)
		if err == nil {
			return address.NewIDAddress(id)
		}
	}

	return address.NewFromString(s)
}

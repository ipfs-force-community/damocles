package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/signal"
	"reflect"
	"strconv"
	"syscall"
	"time"

	"github.com/dtynn/dix"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/urfave/cli/v2"
	"go.uber.org/fx"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/dep"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/homedir"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/messager"
)

const logSubSystem = "cmd"

var Log = logging.New(logSubSystem)

var HomeFlag = &cli.StringFlag{
	Name:    "home",
	Value:   "~/.venus-sector-manager",
	EnvVars: []string{"DAMOCLES_PATH", "VENUS_SECTOR_MANAGER_PATH", "VSM_PATH"},
}

// Note: NetFlag is deprecated and will be removed in a future version.
var NetFlag = &cli.StringFlag{
	Name:  "net",
	Usage: "DEPRECATED: venus-sector-manager will automatically get the network parameters",
}

var SealerListenFlag = &cli.StringFlag{
	Name:  "listen",
	Value: ":1789",
}

var ConfDirFlag = &cli.StringFlag{
	Name:    "conf-dir",
	Usage:   "the dir path in which the sector-manager.cfg file exists, set this only if you don't want to use the config file inside home dir",
	EnvVars: []string{"DAMOCLES_CONF_DIR", "VENUS_SECTOR_MANAGER_CONF_DIR", "VSM_CONF_DIR"},
}

type stopper = func()

func NewSigContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGABRT, syscall.SIGTERM, syscall.SIGINT)
}

func DepsFromCLICtx(cctx *cli.Context) dix.Option {
	confDir := cctx.String(ConfDirFlag.Name)
	return dix.Options(
		dix.Override(new(*cli.Context), cctx),
		dix.Override(new(*homedir.Home), HomeFromCLICtx),
		dix.If(confDir != "",
			dix.Override(new(dep.ConfDirPath), func() (dep.ConfDirPath, error) {
				dir, err := homedir.Expand(confDir)
				if err != nil {
					return "", fmt.Errorf("expand conf dir path: %w", err)
				}

				return dep.ConfDirPath(dir), nil
			}),
		),
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

type APIClient struct {
	fx.In
	Chain    chain.API
	Messager messager.API
	Market   market.API
	Sealer   core.SealerCliClient
	Miner    core.MinerAPIClient
}

func extractAPI(cctx *cli.Context, target ...interface{}) (*APIClient, context.Context, stopper, error) {
	gctx, gcancel := NewSigContext(cctx.Context)

	var a APIClient
	wants := append([]interface{}{&a}, target...)

	stopper, err := dix.New(
		gctx,
		dep.APIClient(wants...),
		DepsFromCLICtx(cctx),
		dix.Override(new(dep.GlobalContext), gctx),
		dix.Override(new(dep.ListenAddress), dep.ListenAddress(cctx.String(SealerListenFlag.Name))),
	)

	if err != nil {
		gcancel()
		return nil, nil, nil, fmt.Errorf("construct api: %w", err)
	}

	return &a, gctx, func() {
		stopper(cctx.Context) // nolint: errcheck
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

func ShouldActor(s string, checkEmpty bool) (abi.ActorID, error) {
	addr, err := ShouldAddress(s, checkEmpty, true)
	if err != nil {
		return 0, err
	}

	actor, err := address.IDFromAddress(addr)
	if err != nil {
		return 0, fmt.Errorf("get actor id from addr: %w", err)
	}

	return abi.ActorID(actor), nil
}

func ShouldSectorNumber(s string) (abi.SectorNumber, error) {
	num, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse sector number: %w", err)
	}

	return abi.SectorNumber(num), nil
}

func waitMessage(ctx context.Context, api *APIClient, rawMsg *messager.UnsignedMessage, exid string, mlog *logging.ZapLogger, ret cbor.Unmarshaler) error {
	if mlog == nil {
		mlog = Log.With("action", "wait-message")
	}

	mblk, err := rawMsg.ToStorageBlock()
	if err != nil {
		return fmt.Errorf("failed to generate msg node: %w", err)
	}

	mid := mblk.Cid().String()
	if exid != "" {
		mid = fmt.Sprintf("%s-%s", mid, exid)
		mlog = mlog.With("exid", exid)
		mlog.Warnf("use extra message id")
	}

	has, err := api.Messager.HasMessageByUid(ctx, mid)
	if err != nil {
		return RPCCallError("HasMessageByUid", err)
	}

	if !has {
		rmid, err := api.Messager.PushMessageWithId(ctx, mid, rawMsg, &messager.MsgMeta{})
		if err != nil {
			return RPCCallError("PushMessageWithId", err)
		}

		if rmid != mid {
			mlog.Warnf("mcid not equal to recv id: %s != %s", mid, rmid)
		}
	}

	mlog = mlog.With("mid", mid)
	var mret *messager.Message
WAIT_RET:
	for {
		mlog.Info("wait for message receipt")
		time.Sleep(30 * time.Second)

		ret, err := api.Messager.GetMessageByUid(ctx, mid)
		if err != nil {
			mlog.Warnf("GetMessageByUid: %s", err)
			continue
		}

		switch ret.State {
		case messager.MessageState.OnChainMsg, messager.MessageState.NonceConflictMsg:
			mret = ret
			break WAIT_RET

		default:
			mlog.Infof("msg state: %s", messager.MessageStateToString(ret.State))
		}
	}

	mlog = mlog.With("smcid", mret.SignedCid.String(), "height", mret.Height)

	mlog.Infow("message landed on chain")
	if mret.Receipt.ExitCode != 0 {
		mlog.Warnf("message exec failed: %s(%d)", mret.Receipt.ExitCode, mret.Receipt.ExitCode)
		return nil
	}

	if ret != nil {
		if err := ret.UnmarshalCBOR(bytes.NewReader(mret.Receipt.Return)); err != nil {
			return fmt.Errorf("unmarshal message return: %w", err)
		}
	}

	return nil
}

func OutputJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "\t")
	return enc.Encode(v)
}

func FormatOrNull(arg interface{}, f func() string) string {
	if arg == nil {
		return "NULL"
	}

	rv := reflect.ValueOf(arg)
	rt := rv.Type()
	if rt.Kind() == reflect.Ptr && rv.IsNil() {
		return "NULL"

	}

	return f()
}

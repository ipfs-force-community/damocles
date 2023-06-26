package sectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/filecoin-project/go-state-types/abi"
	gtypes "github.com/filecoin-project/venus/venus-shared/types/gateway"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

var unsealInfoKey = kvstore.Key("unseal-infos")

const invalidMarketHost = "invalid.market.host"

type Key = string
type UnsealInfos struct {
	AllocIndex map[abi.ActorID]map[Key]struct{}
	Data       map[Key]*core.SectorUnsealInfo
}

// UnsealManager manage unseal task
type UnsealManager struct {
	msel        *minerSelector
	kvMu        sync.Mutex
	kv          kvstore.KVStore
	defaultDest *url.URL
	onAchieve   map[Key][]func()
}

var _ core.UnsealSectorManager = (*UnsealManager)(nil)

func NewUnsealManager(ctx context.Context, scfg *modules.SafeConfig, minfoAPI core.MinerAPI, kv kvstore.KVStore) (ret *UnsealManager, err error) {
	ret = &UnsealManager{
		kv:   kv,
		msel: newMinerSelector(scfg, minfoAPI),
	}

	scfg.Lock()
	commonAPI := scfg.Config.Common.API
	scfg.Unlock()
	ret.defaultDest, err = getDefaultMarketPiecesStore(commonAPI.Market)
	if err != nil {
		log.Warnw("get default market pieces store fail, upload unseal piece to market will not be possible", "error", err)
	}
	q := ret.defaultDest.Query()
	q.Set("token", commonAPI.Token)
	ret.defaultDest.RawQuery = q.Encode()

	return ret, nil

}

// SetAndCheck set unseal task
func (u *UnsealManager) SetAndCheck(ctx context.Context, req *core.SectorUnsealInfo) (state gtypes.UnsealState, err error) {

	// check dest url
	supplied, err := u.checkDestUrl(req.Dest[0])
	if err != nil {
		return gtypes.UnsealStateFailed, err
	}
	req.Dest[0] = supplied

	log := log.With("dest", supplied)

	// set into db
	key := core.UnsealInfoKey(req.Sector.ID.Miner, req.Sector.ID.Number, req.PieceCid)
	log = log.With("key", key)

	// error can be happen when set task into db (except the error from db)
	var updateErr error
	err = u.loadAndUpdate(ctx, func(db *UnsealInfos) bool {
		info, ok := db.Data[key]
		if !ok {
			//not found, add new task
			req.State = gtypes.UnsealStateSet
			db.Data[key] = req

			// build index
			keySet, ok := db.AllocIndex[req.Sector.ID.Miner]
			if !ok {
				keySet = map[Key]struct{}{}
			}
			keySet[key] = struct{}{}
			db.AllocIndex[req.Sector.ID.Miner] = keySet
			log = log.With("state", req.State)
			log.Info("add new unseal task")
			return true
		}
		state = info.State
		log = log.With("state", state)

		//  if task exit, check dest
		haveSet := false
		for _, v := range info.Dest {
			if v == supplied {
				haveSet = true
			}
		}
		if haveSet {
			if state == gtypes.UnsealStateFinished {
				// remove task once finished
				delete(db.Data, key)
				// clear hooks
				delete(u.onAchieve, key)
				log = log.With("state", state)
				log.Info("finished unseal task has been observe, remove it")
				return true
			}
			log.Info("dest already set")
			return false
		}

		// append new dest is not allow once task is uploading
		if state == gtypes.UnsealStateUploading {
			log.Warn("task is uploading, can't add new dest")
			updateErr = fmt.Errorf("task is uploading, can't add new dest, please try it again later")
			return false
		}

		info.Dest = append(info.Dest, supplied)
		log = log.With("newDest", supplied)
		db.Data[key] = info
		log.Info("add new dest to unseal task")
		return true
	})

	if err != nil {
		return state, fmt.Errorf("check unseal state: %w", err)
	}
	if updateErr != nil {
		return state, fmt.Errorf("update unseal state: %w", updateErr)
	}

	return state, nil
}

// Set set unseal task
func (u *UnsealManager) OnAchieve(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, hook func()) {
	key := core.UnsealInfoKey(sid.Miner, sid.Number, pieceCid)
	u.kvMu.Lock()
	defer u.kvMu.Unlock()
	if u.onAchieve == nil {
		u.onAchieve = map[string][]func(){}
	}
	newHooks := append(u.onAchieve[key], hook)
	u.onAchieve[key] = newHooks
}

// allocate a unseal task
func (u *UnsealManager) Allocate(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorUnsealInfo, error) {
	cands := u.msel.candidates(ctx, spec.AllowedMiners, spec.AllowedProofTypes, func(mcfg modules.MinerConfig) bool { return true }, "unseal")
	if len(cands) == 0 {
		return nil, nil
	}

	// read db
	var allocated *core.SectorUnsealInfo
	err := u.loadAndUpdate(ctx, func(db *UnsealInfos) bool {

		if len(db.AllocIndex) == 0 {
			return false
		}

		for _, candidate := range cands {
			keySet, ok := db.AllocIndex[candidate.info.ID]
			if !ok {
				continue
			}
			if len(keySet) == 0 {
				continue
			}
			for k := range keySet {
				allocated = db.Data[k]
				// todo: exclude online sector
				key := core.UnsealInfoKey(allocated.Sector.ID.Miner, allocated.Sector.ID.Number, allocated.PieceCid)
				delete(db.AllocIndex[candidate.info.ID], k)
				allocated.Sector.ProofType = candidate.info.SealProofType
				allocated.State = gtypes.UnsealStateUnsealing
				db.Data[key] = allocated
				return true
			}
		}
		return false
	})

	if err != nil {
		return nil, fmt.Errorf("allocate unseal info: %w", err)
	}

	return allocated, nil
}

// achieve a unseal task
func (u *UnsealManager) Achieve(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, unsealErr string) error {
	key := core.UnsealInfoKey(sid.Miner, sid.Number, pieceCid)

	var info *core.SectorUnsealInfo
	err := u.loadAndUpdate(ctx, func(db *UnsealInfos) bool {
		ok := false
		info, ok = db.Data[key]
		if !ok {
			return false
		}
		info.State = gtypes.UnsealStateFinished
		db.Data[key] = info
		return true
	})

	if err != nil {
		return fmt.Errorf("achieve unseal info(%s): %w", key, err)
	}

	if info == nil {
		return fmt.Errorf("achieve unseal info(%s): task not found", key)
	}

	// call hook
	hooks, ok := u.onAchieve[key]
	if ok {
		for _, hook := range hooks {
			hook()
		}
		delete(u.onAchieve, key)
	}

	return nil
}

func (u *UnsealManager) AcquireDest(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid) ([]string, error) {
	key := core.UnsealInfoKey(sid.Miner, sid.Number, pieceCid)

	var dest []string
	err := u.loadAndUpdate(ctx, func(db *UnsealInfos) bool {
		info, ok := db.Data[key]
		if !ok {
			log.Errorf("acquire unseal info(%s):record not found", key)
		}
		dest = info.Dest
		info.State = gtypes.UnsealStateUploading
		db.Data[key] = info
		return true

	})

	if err != nil {
		return nil, fmt.Errorf("achieve unseal info(%s): %w", key, err)
	}

	return dest, nil
}

func (u *UnsealManager) loadAndUpdate(ctx context.Context, modify func(infos *UnsealInfos) bool) error {
	u.kvMu.Lock()
	defer u.kvMu.Unlock()

	var infos UnsealInfos
	err := u.kv.Peek(ctx, unsealInfoKey, func(v kvstore.Val) error {
		verr := json.Unmarshal(v, &infos)
		if verr != nil {
			return fmt.Errorf("unmashal unseal infos: %w", verr)
		}

		return nil
	})

	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return fmt.Errorf("load unseal infos: %w", err)
		}
	}

	if infos.AllocIndex == nil {
		infos.AllocIndex = make(map[abi.ActorID]map[Key]struct{})
	}
	if infos.Data == nil {
		infos.Data = make(map[string]*core.SectorUnsealInfo)
	}

	updated := modify(&infos)
	if !updated {
		return nil
	}
	allocatable := 0
	for _, v := range infos.AllocIndex {
		allocatable += len(v)
	}
	log.Debugf("update unseal task: Allocatable(%d) Allocated(%d)", allocatable, len(infos.Data)-allocatable)

	val, err := json.Marshal(infos)
	if err != nil {
		return fmt.Errorf("marshal unseal infos: %w", err)
	}

	err = u.kv.Put(ctx, unsealInfoKey, val)
	if err != nil {
		return fmt.Errorf("put data of unseal infos: %w", err)
	}

	return nil
}

// checkDestUrl check dest url conform to the out expect
// we accept three kinds of url by now
// 1. http://xxx or https://xxx , it means we will put data to a http server
// 2. market://store_name/piece_cid, it means we will put data to the target path of pieces store from market
// 3. file:///path , it means we will put data to a local file
// 4. store://store_name/piece_cid , it means we will put data to damocles-manager
func (u *UnsealManager) checkDestUrl(dest string) (string, error) {
	urlStruct, err := url.Parse(dest)
	if err != nil {
		return "", err
	}

	switch urlStruct.Scheme {
	case "http", "https", "file", "store":
	case "market":
		if u.defaultDest.Host == invalidMarketHost {
			return "", fmt.Errorf("upload pieces to market will not be possible when market address dose not set, please check you config")
		}
		// add host , scheme and token
		q := urlStruct.Query()
		q.Set("token", u.defaultDest.Query().Get("token"))
		q.Set("host", u.defaultDest.Host)
		q.Set("scheme", u.defaultDest.Scheme)
		urlStruct.RawQuery = q.Encode()
	default:
		return "", fmt.Errorf("invalid dest url: %s(unsupported scheme %s)", dest, urlStruct.Scheme)
	}

	return urlStruct.String(), nil
}

type MultiAddr = string

// getDefaultMarketPiecesStore get the default pieces store url from market api
func getDefaultMarketPiecesStore(marketAPI MultiAddr) (*url.URL, error) {
	ret := &url.URL{
		Scheme: "http",
		Host:   invalidMarketHost,
	}

	ma, err := multiaddr.NewMultiaddr(marketAPI)
	if err != nil {
		return ret, fmt.Errorf("parse market api fail %w", err)
	}

	_, addr, err := manet.DialArgs(ma)
	if err != nil {
		return ret, fmt.Errorf("parse market api fail %w", err)
	}
	ret.Host = addr

	_, err = ma.ValueForProtocol(multiaddr.P_WSS)
	if err == nil {
		ret.Scheme = "https"
	}

	_, err = ma.ValueForProtocol(multiaddr.P_HTTPS)
	if err == nil {
		ret.Scheme = "https"
	}

	ret.Path = "/resource"
	return ret, nil
}

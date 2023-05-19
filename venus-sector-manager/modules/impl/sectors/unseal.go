package sectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	vtypes "github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

var unsealInfoKey = kvstore.Key("unseal-infos")

type UnsealInfos struct {
	Allocatable map[abi.ActorID]map[string]*core.SectorUnsealInfo
	Allocated   map[string]*core.SectorUnsealInfo
}

// UnsealManager manage unseal task
type UnsealManager struct {
	msel        *minerSelector
	kvMu        sync.Mutex
	kv          kvstore.KVStore
	defaultDest *url.URL
	marketEvent market.IMarketEvent
	onAchieve   map[string]func()
}

var _ core.UnsealSectorManager = (*UnsealManager)(nil)

func NewUnsealManager(ctx context.Context, scfg *modules.SafeConfig, minfoAPI core.MinerAPI, kv kvstore.KVStore, mEvent market.IMarketEvent) (ret *UnsealManager, err error) {
	ret = &UnsealManager{
		kv:          kv,
		msel:        newMinerSelector(scfg, minfoAPI),
		marketEvent: mEvent,
	}

	scfg.Lock()
	commonAPI := scfg.Config.Common.API
	scfg.Unlock()
	ret.defaultDest, err = getDefaultMarketPiecesStore(commonAPI.Market)
	if err != nil {
		return nil, fmt.Errorf("get default market pieces store: %w", err)
	}
	q := ret.defaultDest.Query()
	q.Set("token", commonAPI.Token)
	ret.defaultDest.RawQuery = q.Encode()

	if err != nil {
		return
	}

	// register to market event
	mEvent.OnUnseal(func(ctx context.Context, eventId vtypes.UUID, req *market.UnsealRequest) {
		actor, err := address.IDFromAddress(req.Miner)
		if err != nil {
			log.Errorf("get miner id from address: %s", err)
			return
		}
		err = ret.Set(ctx, &core.SectorUnsealInfo{
			Sector: core.AllocatedSector{
				ID: abi.SectorID{
					Miner:  abi.ActorID(actor),
					Number: req.Sid,
				},
				// proofType should be set when allocate
			},
			PieceCid: req.PieceCid,
			Offset:   uint64(req.Offset),
			Size:     uint64(req.Size),
			Dest:     req.Dest,
			EventIds: []vtypes.UUID{eventId},
		})
		if err != nil {
			log.Errorf("set unseal info: %s", err)
		}
	})
	return ret, nil

}

// Set set unseal task
func (u *UnsealManager) Set(ctx context.Context, req *core.SectorUnsealInfo) (err error) {
	// check piece store
	// todo: if exist in piece store , respond directly

	// check dest url
	req.Dest, err = u.checkDestUrl(req.Dest)
	if err != nil {
		return err
	}

	// set into db
	err = u.loadAndUpdate(ctx, func(infos *UnsealInfos) bool {
		key := core.UnsealInfoKey(req.Sector.ID.Miner, req.Sector.ID.Number, req.PieceCid)
		info, ok := infos.Allocatable[req.Sector.ID.Miner]
		if !ok {
			info = map[string]*core.SectorUnsealInfo{}
		}

		//  if task exit, merge event id
		if before, ok := info[key]; ok {
			log.Infof("update unseal info (%s) with %d event id ", key, len(req.EventIds))
			req.EventIds = append(req.EventIds, before.EventIds...)
			infos.Allocated[key] = req
		}

		info[key] = req
		infos.Allocatable[req.Sector.ID.Miner] = info
		log.Infof("set unseal info: %s , ", key)
		return true
	})

	if err != nil {
		return fmt.Errorf("add unseal info: %w", err)
	}
	return nil
}

// Set set unseal task
func (u *UnsealManager) OnAchieve(ctx context.Context, sid abi.SectorID, pieceCid cid.Cid, hook func()) {
	key := core.UnsealInfoKey(sid.Miner, sid.Number, pieceCid)
	u.kvMu.Lock()
	defer u.kvMu.Unlock()
	if u.onAchieve == nil {
		u.onAchieve = map[string]func(){}
	}
	u.onAchieve[key] = hook
}

// allocate a unseal task
func (u *UnsealManager) Allocate(ctx context.Context, spec core.AllocateSectorSpec) (*core.SectorUnsealInfo, error) {
	cands := u.msel.candidates(ctx, spec.AllowedMiners, spec.AllowedProofTypes, func(mcfg modules.MinerConfig) bool { return true }, "unseal")
	if len(cands) == 0 {
		return nil, nil
	}

	// read db
	var allocated *core.SectorUnsealInfo
	err := u.loadAndUpdate(ctx, func(infos *UnsealInfos) bool {

		if len(infos.Allocatable) == 0 {
			return false
		}

		for _, candidate := range cands {
			info, ok := infos.Allocatable[candidate.info.ID]
			if !ok {
				continue
			}
			if len(info) == 0 {
				continue
			}
			for sectorNum, v := range info {
				allocated = v
				// todo: exclude online sector
				key := core.UnsealInfoKey(allocated.Sector.ID.Miner, allocated.Sector.ID.Number, allocated.PieceCid)
				delete(infos.Allocatable[candidate.info.ID], sectorNum)
				allocated.Sector.ProofType = candidate.info.SealProofType
				infos.Allocated[key] = allocated
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
	// respond to market
	var info *core.SectorUnsealInfo
	err := u.loadAndUpdate(ctx, func(infos *UnsealInfos) bool {
		ok := false
		info, ok = infos.Allocated[key]
		if !ok {
			return false
		}
		delete(infos.Allocated, key)
		return true
	})

	if err != nil {
		return fmt.Errorf("achieve unseal info(%s): %w", key, err)
	}

	if info == nil {
		return fmt.Errorf("achieve unseal info(%s): task not found", key)
	}

	// call hook
	hook, ok := u.onAchieve[key]
	if ok {
		hook()
	}

	// respond to market
	for _, eventID := range info.EventIds {
		err := u.marketEvent.RespondUnseal(ctx, eventID, unsealErr)
		if err != nil {
			log.Errorf("respond unseal event: %s", err)
		}
	}

	return nil
}

func (u *UnsealManager) loadAndUpdate(ctx context.Context, modify func(infos *UnsealInfos) bool) error {
	u.kvMu.Lock()
	defer u.kvMu.Unlock()

	var infos UnsealInfos
	err := u.kv.View(ctx, unsealInfoKey, func(v kvstore.Val) error {
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

	if infos.Allocatable == nil {
		infos.Allocatable = make(map[abi.ActorID]map[string]*core.SectorUnsealInfo)
	}
	if infos.Allocated == nil {
		infos.Allocated = make(map[string]*core.SectorUnsealInfo)
	}

	updated := modify(&infos)
	if !updated {
		return nil
	}
	log.Debugf("update unseal task: Allocatable(%d) Allocated(%d)", len(infos.Allocatable), len(infos.Allocated))

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
// 4. store://store_name/piece_cid , it means we will put data to store in venus-sector-manager
func (u *UnsealManager) checkDestUrl(dest string) (string, error) {
	urlStruct, err := url.Parse(dest)
	if err != nil {
		return "", err
	}

	switch urlStruct.Scheme {
	case "http", "https", "file", "store":
	case "market":
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
	}

	ma, err := multiaddr.NewMultiaddr(marketAPI)
	if err != nil {
		return nil, fmt.Errorf("parse market api fail %w", err)
	}

	_, addr, err := manet.DialArgs(ma)
	if err != nil {
		return nil, fmt.Errorf("parse market api fail %w", err)
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

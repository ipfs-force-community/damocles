package sectors

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"sort"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/patrickmn/go-cache"
	"github.com/samber/lo"
	"golang.org/x/sync/singleflight"
)

func NewSenderSelector(chain chain.API) *SenderSelector {
	cache := cache.New(time.Duration(policy.NetParams.BlockDelaySecs/2), time.Duration(policy.NetParams.BlockDelaySecs))
	return &SenderSelector{chain: chain, cache: cache}
}

type SenderSelector struct {
	chain chain.API
	cache *cache.Cache
	group singleflight.Group
}

func (s *SenderSelector) Select(ctx context.Context, mid abi.ActorID, senders []address.Address) (address.Address, error) {
	key := cacheKey(mid, senders)
	sender, found := s.cache.Get(key)
	if !found {
		var err error
		sender, err, _ = s.group.Do(key, func() (interface{}, error) {
			senderInner, errInner := s.selectInner(ctx, mid, senders)
			if err == nil {
				s.cache.Set(key, sender, cache.DefaultExpiration)
			}
			return senderInner, errInner
		})
		if err != nil {
			return address.Undef, err
		}
	}
	log.Infof("selected sender: %s", sender)
	return sender.(address.Address), nil
}

func (s *SenderSelector) selectInner(ctx context.Context, mid abi.ActorID, senders []address.Address) (address.Address, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return address.Undef, err
	}
	minerInfo, err := s.chain.StateMinerInfo(ctx, maddr, types.EmptyTSK)
	if err != nil {
		return address.Undef, fmt.Errorf("get miner info for %s: %w", maddr, err)
	}
	minerAddrs := lo.Flatten([][]address.Address{{minerInfo.Owner, minerInfo.Worker}, minerInfo.ControlAddresses})
	validAddrs := lo.Intersect(senders, minerAddrs)

	if len(validAddrs) == 0 {
		return address.Undef, fmt.Errorf("no valid sender found")
	}

	if len(validAddrs) > 1 {
		balancesCache := make(map[address.Address]types.BigInt)
		sort.Slice(validAddrs, func(i, j int) bool {
			balanceI := valueOrInsert(balancesCache, validAddrs[i], func() types.BigInt {
				return s.getBalance(ctx, validAddrs[i], big.Zero())
			})
			balanceJ := valueOrInsert(balancesCache, validAddrs[j], func() types.BigInt {
				return s.getBalance(ctx, validAddrs[j], big.Zero())
			})
			return balanceI.GreaterThan(balanceJ)
		})
	}
	return validAddrs[0], nil
}

func (s *SenderSelector) getBalance(ctx context.Context, addr address.Address, defaultBalance types.BigInt) types.BigInt {
	actor, err := s.chain.StateGetActor(ctx, addr, types.EmptyTSK)
	if err != nil {
		log.Warnf("get actor for: %s, %w", addr, err)
		return defaultBalance
	}

	return actor.Balance
}

func cacheKey(mid abi.ActorID, senders []address.Address) string {
	h := xxhash.New()
	midBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(midBytes, uint64(mid))
	// It always returns len(b), nil.
	_, _ = h.Write(midBytes)
	for _, sender := range senders {
		_, _ = h.Write(sender.Bytes())
	}
	key := make([]byte, 8)
	binary.LittleEndian.PutUint64(key, h.Sum64())
	return base64.StdEncoding.EncodeToString(key)
}

// valueOrInsert returns the value of the given key or insert the fallback value into map if the key is not present.
func valueOrInsert[K comparable, V any](in map[K]V, key K, fallback func() V) V {
	if v, ok := in[key]; ok {
		return v
	}
	in[key] = fallback()
	return in[key]
}

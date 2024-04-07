package address

import (
	"context"
	"fmt"
	"sync"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"golang.org/x/sync/singleflight"
)

var log = logging.New("address")

type ChainAPI interface {
	StateLookupID(ctx context.Context, addr address.Address, tsk types.TipSetKey) (address.Address, error)
}

func NewCacheableLookupID(api ChainAPI) *CacheableLookupID {
	return &CacheableLookupID{
		chain:       api,
		toF0mapping: make(map[address.Address]address.Address),
		mu:          &sync.RWMutex{},
	}
}

type CacheableLookupID struct {
	chain       ChainAPI
	group       singleflight.Group
	toF0mapping map[address.Address]address.Address
	mu          *sync.RWMutex
}

func (l *CacheableLookupID) StateLookupID(ctx context.Context, nonF0 address.Address) (address.Address, error) {
	l.mu.RLock()
	f0, has := l.toF0mapping[nonF0]
	l.mu.RUnlock()
	if has {
		return f0, nil
	}

	key := nonF0.String()
	idAddr, err, _ := l.group.Do(key, func() (interface{}, error) {
		idAddr, err := l.chain.StateLookupID(ctx, nonF0, types.EmptyTSK)
		if err != nil {
			return address.Undef, fmt.Errorf("convert non-f0 address to f0 address %s: %w", nonF0, err)
		}
		return idAddr, err
	})

	if err != nil {
		return address.Undef, err
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	l.toF0mapping[nonF0] = idAddr.(address.Address)
	return l.toF0mapping[nonF0], nil
}

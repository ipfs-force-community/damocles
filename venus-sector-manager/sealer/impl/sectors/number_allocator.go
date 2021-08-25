package sectors

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/kvstore"
	"github.com/dtynn/venus-cluster/venus-sector-manager/sealer/api"
)

var _ api.SectorNumberAllocator = (*NumberAllocator)(nil)

func NewNumerAllocator(store kvstore.KVStore) (*NumberAllocator, error) {
	return &NumberAllocator{
		store:  store,
		locker: newSectorsLocker(),
	}, nil
}

type NumberAllocator struct {
	store kvstore.KVStore

	locker *sectorsLocker
}

func (na *NumberAllocator) Next(ctx context.Context, mid abi.ActorID) (uint64, error) {
	lock := na.locker.lock(abi.SectorID{
		Miner:  mid,
		Number: 0,
	})

	defer lock.unlock()

	key := []byte(fmt.Sprintf("/m-%d", mid))
	var current uint64
	switch err := na.store.View(ctx, key, func(data []byte) error {
		num, read := binary.Uvarint(data)
		if read != len(data) {
			return fmt.Errorf("raw data is not a valid uvarint: %v", data)
		}

		current = num
		return nil
	}); err {
	case nil:

	case kvstore.ErrKeyNotFound:

	default:
		return 0, fmt.Errorf("fetch current number for %d: %w", mid, err)
	}

	current++
	data := make([]byte, binary.MaxVarintLen64)
	written := binary.PutUvarint(data, current)
	if err := na.store.Put(ctx, key, data[:written]); err != nil {
		return 0, fmt.Errorf("write next seq number: %w", err)
	}

	return current, nil
}

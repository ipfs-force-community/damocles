package sectors

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/filecoin-project/go-state-types/abi"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/kvstore"
)

var _ core.SectorNumberAllocator = (*NumberAllocator)(nil)

func NewNumberAllocator(store kvstore.KVStore) (*NumberAllocator, error) {
	return &NumberAllocator{
		store:  store,
		locker: newSectorsLocker(),
	}, nil
}

type NumberAllocator struct {
	store kvstore.KVStore

	locker *sectorsLocker
}

func (na *NumberAllocator) NextN(
	ctx context.Context,
	mid abi.ActorID,
	n uint32,
	minNum uint64,
	check func(uint64) bool,
) (uint64, bool, error) {
	lock := na.locker.lock(abi.SectorID{
		Miner:  mid,
		Number: 0,
	})

	defer lock.unlock()

	key := []byte(fmt.Sprintf("/m-%d", mid))
	var current uint64
	switch err := na.store.Peek(ctx, key, func(data []byte) error {
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
		return 0, false, fmt.Errorf("fetch current number for %d: %w", mid, err)
	}

	if current < minNum {
		current = minNum
	}

	current += uint64(n)
	if !check(current) {
		return current, false, nil
	}

	data := make([]byte, binary.MaxVarintLen64)
	written := binary.PutUvarint(data, current)
	if err := na.store.Put(ctx, key, data[:written]); err != nil {
		return 0, false, fmt.Errorf("write next seq number: %w", err)
	}

	return current, true, nil
}

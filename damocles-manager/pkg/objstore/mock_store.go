package objstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

var _ Store = (*MockStore)(nil)

func NewMockStore(cfg Config, size uint64) (Store, error) {
	if size == 0 {
		return nil, fmt.Errorf("zero-sized store is not valid")
	}

	return &MockStore{
		cfg:     cfg,
		size:    size,
		used:    0,
		objects: map[string]*bytes.Buffer{},
	}, nil
}

type MockStore struct {
	cfg     Config
	size    uint64
	used    uint64
	objects map[string]*bytes.Buffer
	mu      sync.RWMutex
}

func (*MockStore) Type() string {
	return "mock"
}

func (*MockStore) Version() string {
	return "mock"
}

func (ms *MockStore) Instance(context.Context) string {
	return ms.cfg.Name
}

func (ms *MockStore) InstanceInfo(context.Context) (InstanceInfo, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return InstanceInfo{
		Config:      ms.cfg,
		Type:        "mock",
		Total:       ms.size,
		Free:        ms.size - ms.used,
		Used:        ms.used,
		UsedPercent: float64(ms.used) * 100 / float64(ms.size),
	}, nil
}

func (ms *MockStore) InstanceConfig(context.Context) Config {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.cfg
}

func (ms *MockStore) Get(_ context.Context, p string) (io.ReadCloser, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	buf, ok := ms.objects[p]
	if !ok {
		return nil, ErrObjectNotFound
	}

	r := new(bytes.Buffer)
	_, err := io.Copy(r, buf)
	if err != nil {
		return nil, err
	}

	return io.NopCloser(r), nil
}

func (ms *MockStore) Del(_ context.Context, p string) error {
	if ms.cfg.ReadOnly {
		return ErrReadOnlyStore
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	buf, ok := ms.objects[p]
	if !ok {
		return ErrObjectNotFound
	}

	released := uint64(buf.Len())
	ms.used -= released
	delete(ms.objects, p)
	return nil
}

func (ms *MockStore) Stat(_ context.Context, p string) (Stat, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	buf, ok := ms.objects[p]
	if !ok {
		return Stat{}, ErrObjectNotFound
	}

	return Stat{
		Size: int64(buf.Len()),
	}, nil
}

func (ms *MockStore) Put(_ context.Context, p string, r io.Reader) (int64, error) {
	if ms.cfg.ReadOnly {
		return 0, ErrReadOnlyStore
	}

	buf := new(bytes.Buffer)
	written, err := io.Copy(buf, r)
	if err != nil {
		return 0, fmt.Errorf("copy data: %w", err)
	}

	size := uint64(written)

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if exist, ok := ms.objects[p]; ok {
		delete(ms.objects, p)
		ms.used -= uint64(exist.Len())
	}

	if ms.used+size > ms.size {
		return 0, fmt.Errorf("not enough space, %d requreid, %d remain", size, ms.size-ms.used)
	}

	ms.objects[p] = buf
	ms.used += size

	return written, nil
}

func (*MockStore) FullPath(_ context.Context, p string) string {
	return p
}

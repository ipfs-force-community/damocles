package objstore

import (
	"context"
	"fmt"
	"io"
)

var (
	ErrNotRegularFile              = fmt.Errorf("not a regular file")
	ErrNotSeekable                 = fmt.Errorf("not seekable")
	ErrNotOpened                   = fmt.Errorf("not opened")
	ErrReadOnlyStore               = fmt.Errorf("read only store")
	ErrInvalidObjectPath           = fmt.Errorf("invalid object path")
	ErrObjectStoreInstanceNotFound = fmt.Errorf("instance not found")
	ErrObjectNotFound              = fmt.Errorf("object not found")
)

type Config struct {
	Name string
	Path string
	Meta map[string]string

	// Strict should never be used directly, use GetStrict() instead
	Strict *bool
	// ReadOnly should never be used directly, use GetReadOnly() instead
	ReadOnly *bool
	// Weight should never be used directly, use GetWeight() instead
	Weight *uint
}

func (c Config) GetStrict() bool {
	if c.Strict == nil {
		return false
	}
	return *c.Strict
}

func (c Config) GetReadOnly() bool {
	if c.ReadOnly == nil {
		return false
	}
	return *c.ReadOnly
}

func (c Config) GetWeight() uint {
	if c.Weight == nil {
		return 1
	}
	return *c.Weight
}

func DefaultConfig(path string, readonly bool) Config {
	one := uint(1)
	return Config{
		Path:     path,
		Meta:     map[string]string{},
		ReadOnly: &readonly,
		Weight:   &one,
	}
}

type Stat struct {
	Size int64 // file size
}

type InstanceInfo struct {
	Config      Config
	Type        string  // the type of this store, just for display
	Total       uint64  // the total capacity of this store, in bytes
	Free        uint64  // the free capacity of this store, in bytes
	Used        uint64  // the used capacity of this store, in bytes
	UsedPercent float64 // the percentage of used capacity for this store.
}

type Store interface {
	// Type returns the type of this store, just for display.
	Type() string
	// Type returns the version of this store, just for display.
	Version() string
	Instance(context.Context) string
	// InstanceConfig returns the configuration of this store.
	InstanceConfig(ctx context.Context) Config
	// InstanceInfo returns the information of this store.
	InstanceInfo(ctx context.Context) (InstanceInfo, error)

	// Get returns an io.ReadCloser object for reading the file corresponding to the fullPath from this store.
	Get(ctx context.Context, fullPath string) (io.ReadCloser, error)
	// Del removes the file corresponding to given path from this store
	Del(ctx context.Context, fullPath string) error
	// Stat returns a Stat describing the named file
	Stat(ctx context.Context, fullPath string) (Stat, error)
	// Put copies from src to dstPath until either EOF is reached
	// on src or an error occurs. It returns the number of bytes
	// copied.
	Put(ctx context.Context, dstFullPath string, src io.Reader) (int64, error)

	// FullPath returns the full path of the given path.
	//
	// Example:
	// ```go
	// assert(FullPath(context.TODO(), "cache/s-t04040-1001") == "/depends_on_your_store/cache/s-t04040-1001")
	// ```
	FullPath(ctx context.Context, path string) string
}

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

	Strict   bool
	ReadOnly bool
	Weight   uint
}

func DefaultConfig(path string, readonly bool) Config {
	return Config{
		Path:     path,
		Meta:     map[string]string{},
		ReadOnly: readonly,
		Weight:   1,
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
	// Version returns the version of this store, just for display.
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
	// Stat returns a Stat describing the file corresponding to given path
	Stat(ctx context.Context, fullPath string) (Stat, error)
	// Put copies from src to dstPath until either EOF is reached
	// on src or an error occurs.
	// It returns the number of bytes copied.
	Put(ctx context.Context, dstFullPath string, src io.Reader) (int64, error)

	// FullPath returns the full path of the given relative path.
	//
	// ```go
	// assert(FullPath(context.TODO(), "cache/s-t04040-1001") == "depends_on_your_store/cache/s-t04040-1001")
	// ```
	//
	// Example(for filesystem):
	// ```go
	// assert(FullPath(context.TODO(), "cache/s-t04040-1001") == "/path/to/your_store_dir/cache/s-t04040-1001")
	// ```
	FullPath(ctx context.Context, relPath string) string
}

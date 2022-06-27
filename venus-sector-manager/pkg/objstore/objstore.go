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

type CompactConfig struct {
	Name string
	Path string
}

func (cc CompactConfig) ToConfig() Config {
	return Config{
		CompactConfig: cc,
	}
}

type Config struct {
	CompactConfig
	Strict   bool
	ReadOnly bool
	Weight   uint
	Meta     map[string]string
}

func DefaultConfig(path string, readonly bool) Config {
	return Config{
		CompactConfig: CompactConfig{
			Path: path,
		},
		ReadOnly: readonly,
		Weight:   1,
		Meta:     map[string]string{},
	}
}

type ReaderResult struct {
	io.ReadCloser
	Err error
}

type Range struct {
	Offset int64
	Size   int64
}

type Stat struct {
	Size int64
}

type InstanceInfo struct {
	Config      Config
	Type        string
	Total       uint64
	Free        uint64
	Used        uint64
	UsedPercent float64
}

type Store interface {
	Instance(context.Context) string
	InstanceConfig(ctx context.Context) Config
	InstanceInfo(ctx context.Context) (InstanceInfo, error)
	Get(ctx context.Context, p string) (io.ReadCloser, error)
	Del(ctx context.Context, p string) error
	Stat(ctx context.Context, p string) (Stat, error)
	Put(ctx context.Context, p string, r io.Reader) (int64, error)
	FullPath(ctx context.Context, p string) string
}

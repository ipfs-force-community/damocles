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
	Name     string
	Path     string
	Meta     map[string]string
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
	Type() string
	Version() string
	Instance(context.Context) string
	InstanceConfig(ctx context.Context) Config
	InstanceInfo(ctx context.Context) (InstanceInfo, error)
	Get(ctx context.Context, p string) (io.ReadCloser, error)
	Del(ctx context.Context, p string) error
	Stat(ctx context.Context, p string) (Stat, error)
	Put(ctx context.Context, p string, r io.Reader) (int64, error)
	FullPath(ctx context.Context, p string) string
}

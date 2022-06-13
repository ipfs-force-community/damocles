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
	Strict   bool
	ReadOnly bool
	Weight   uint
	Meta     map[string]string
}

func DefaultConfig(path string, readonly bool) Config {
	return Config{
		Path:     path,
		ReadOnly: readonly,
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
	InstanceInfo(context.Context) (InstanceInfo, error)
	Get(context.Context, string) (io.ReadCloser, error)
	Del(context.Context, string) error
	Stat(context.Context, string) (Stat, error)
	Put(context.Context, string, io.Reader) (int64, error)
	GetChunks(context.Context, string, []Range) ([]ReaderResult, error)
	FullPath(context.Context, string) string
}

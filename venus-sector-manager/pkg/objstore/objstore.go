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

type Store interface {
	Instance(context.Context) string
	Get(context.Context, string) (io.ReadCloser, error)
	Del(context.Context, string) error
	Stat(context.Context, string) (Stat, error)
	Put(context.Context, string, io.Reader) (int64, error)
	GetChunks(context.Context, string, []Range) ([]ReaderResult, error)
	FullPath(context.Context, string) string
}

type Manager interface {
	GetInstance(ctx context.Context, name string) (Store, error)
}

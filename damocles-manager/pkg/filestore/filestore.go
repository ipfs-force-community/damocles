package filestore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	plugin "github.com/ipfs-force-community/damocles/manager-plugin/filestore"
)

var log = logging.New("filestore")

// errors
var (
	ErrFileStoreInstanceNotFound = fmt.Errorf("instance not found")
	ErrNotRegularFile            = fmt.Errorf("not a regular file")
	ErrNotSeekable               = fmt.Errorf("not seekable")
	ErrNotOpened                 = fmt.Errorf("not opened")
	ErrReadOnlyStore             = fmt.Errorf("read only store")
	ErrInvalidFilePath           = fmt.Errorf("invalid file path")
	ErrFileNotFound              = fmt.Errorf("file not found")
)

// for store
var (
	DefaultConfig = plugin.DefaultConfig
)

type (
	Config       = plugin.Config
	InstanceInfo = plugin.InstanceInfo
	Store        = plugin.Store
	PathType     = plugin.PathType
	SectorID     = plugin.SectorID
)

var (
	PathTypeSealed      = plugin.PathTypeSealed
	PathTypeUpdate      = plugin.PathTypeUpdate
	PathTypeCache       = plugin.PathTypeCache
	PathTypeUpdateCache = plugin.PathTypeUpdateCache
	PathTypeCustom      = plugin.PathTypeCustom
)

func SectorIDFromAbiSectorID(sid *abi.SectorID) *SectorID {
	if sid == nil {
		return nil
	}

	return &SectorID{
		Miner:  uint64(sid.Miner),
		Number: uint64(sid.Number),
	}
}

type Stat struct {
	Size int64 // file size
}

type Ext struct {
	plugin.Store
	dir fs.FS
}

func NewExt(inner plugin.Store) Ext {
	basePath := inner.InstanceConfig(context.TODO()).Path

	return Ext{
		Store: inner,
		dir:   os.DirFS(basePath),
	}
}

func (s Ext) Read(ctx context.Context, fullPath string) (io.ReadCloser, error) {
	res := s.openWithContext(ctx, fullPath, nil)
	return res.ReadCloser, res.Err
}

func (s Ext) Stat(ctx context.Context, fullPath string) (Stat, error) {
	resCh := make(chan statOrErr, 1)
	go func() {
		defer close(resCh)

		var res statOrErr

		finfo, err := os.Stat(fullPath)
		if err == nil {
			res.Stat.Size = finfo.Size()
		} else {
			res.Err = err
		}

		resCh <- res
	}()

	select {
	case <-ctx.Done():
		return Stat{}, ctx.Err()

	case res := <-resCh:
		return res.Stat, res.Err
	}
}

func (s Ext) Write(ctx context.Context, fullPath string, r io.Reader) (int64, error) {
	if s.InstanceConfig(ctx).GetReadOnly() {
		return 0, ErrReadOnlyStore
	}

	file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("file %s: create %w", fullPath, err)
	}

	defer file.Close()

	return io.Copy(file, r)
}

func (s Ext) Del(ctx context.Context, fullPath string) error {
	if s.InstanceConfig(ctx).GetReadOnly() {
		return ErrReadOnlyStore
	}

	err := os.Remove(fullPath)
	if err != nil {
		return fmt.Errorf("del file(%s): %w", fullPath, err)
	}

	return nil
}

func (s Ext) FullPath(ctx context.Context, pathType PathType, sectorID *abi.SectorID, custom *string) (fullPath string, subPath string, err error) {
	subPath, err = s.SubPath(ctx, pathType, SectorIDFromAbiSectorID(sectorID), custom)
	if err != nil {
		return
	}

	basePath := s.InstanceConfig(ctx).Path
	fullPath, err = filepath.Abs(filepath.Join(basePath, subPath))
	if err != nil {
		err = fmt.Errorf("subPath %s: %w", subPath, ErrInvalidFilePath)
		return
	}

	if !strings.HasPrefix(fullPath, basePath) {
		err = fmt.Errorf("subPath %s: %w: outside of the dir", subPath, ErrInvalidFilePath)
		return
	}

	return
}

func (s Ext) openWithContext(ctx context.Context, p string, r *readRange) readerResult {
	resCh := make(chan readerResult, 1)

	go func() {
		defer close(resCh)

		start := time.Now()
		r, err := s.open(p, r, s.InstanceConfig(ctx).GetStrict())
		dur := time.Since(start)

		select {
		case <-ctx.Done():
			if err == nil {
				r.Close()
				r = nil
				err = ctx.Err()
				log.Warnw("file opened, but context has been canceled", "path", p, "elapsed", dur)
			} else {
				log.Warnw("file opened, but context has been canceled", "path", p, "elapsed", dur, "err", err.Error())
			}

		default:
			log.Debugw("file opened", "path", p, "elapsed", dur)
		}

		if err != nil && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrNotRegularFile)) {
			err = fmt.Errorf("obj %s: %w", p, ErrFileNotFound)
		}

		resCh <- readerResult{
			ReadCloser: r,
			Err:        err,
		}

	}()

	select {
	case <-ctx.Done():
		return readerResult{Err: fmt.Errorf("file %s: %w", p, ctx.Err())}

	case res := <-resCh:
		return res
	}
}

func (s Ext) open(fullPath string, r *readRange, strict bool) (io.ReadCloser, error) {
	file, err := s.dir.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("file %s: open: %w", fullPath, err)
	}

	hold := false
	defer func() {
		if !hold {
			file.Close()
		}
	}()

	if strict {
		stat, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("file %s: get stat: %w", fullPath, err)
		}

		if !stat.Mode().Type().IsRegular() {
			file.Close()
			return nil, fmt.Errorf("file %s: %w", fullPath, ErrNotRegularFile)
		}
	}

	var reader io.ReadCloser = file
	if r != nil {
		seek, ok := file.(io.Seeker)
		if !ok {
			return nil, fmt.Errorf("file %s: %w", fullPath, ErrNotSeekable)
		}

		_, err = seek.Seek(r.Offset, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("file %s: seek to %d: %w", fullPath, r.Offset, err)
		}

		reader = &limitedFile{
			Reader: io.LimitReader(file, r.Size),
			file:   file,
		}
	}

	hold = true
	return reader, nil
}

type statOrErr struct {
	Stat
	Err error
}

type readerResult struct {
	io.ReadCloser
	Err error
}

type readRange struct {
	Offset int64
	Size   int64
}

type limitedFile struct {
	io.Reader
	file fs.File
}

func (lf *limitedFile) Close() error {
	return lf.file.Close()
}

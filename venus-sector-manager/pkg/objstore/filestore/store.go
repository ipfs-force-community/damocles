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

	"github.com/shirou/gopsutil/v3/disk"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore"
)

var log = logging.New("objstore-fs")

var _ objstore.Store = (*Store)(nil)

type statOrErr struct {
	objstore.Stat
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

func OpenStores(cfgs []objstore.Config) ([]objstore.Store, error) {
	stores := make([]objstore.Store, 0, len(cfgs))

	for _, cfg := range cfgs {
		store, err := Open(cfg)
		if err != nil {
			return nil, err
		}

		stores = append(stores, store)
	}

	return stores, nil
}

func Open(cfg objstore.Config) (objstore.Store, error) {
	dirPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("abs path for %s: %w", cfg.Path, err)
	}

	stat, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("stat for %s: %w", dirPath, err)
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("%s is not a dir", dirPath)
	}

	cfg.Path = dirPath
	if cfg.Name == "" {
		cfg.Name = dirPath
	}

	log.Infow("load store", "name", cfg.Name, "path", cfg.Path)
	return &Store{
		cfg: cfg,
		dir: os.DirFS(dirPath),
	}, nil
}

type Store struct {
	cfg objstore.Config
	dir fs.FS
}

type limitedFile struct {
	io.Reader
	file fs.File
}

func (lf *limitedFile) Close() error {
	return lf.file.Close()
}

func (s *Store) openWithContext(ctx context.Context, p string, r *readRange) readerResult {
	resCh := make(chan readerResult, 1)

	go func() {
		defer close(resCh)

		start := time.Now()
		r, err := s.open(p, r)
		dur := time.Since(start)

		select {
		case <-ctx.Done():
			if err == nil {
				r.Close()
				r = nil
				err = ctx.Err()
				log.Warnw("file object opened, but context has been canceled", "path", p, "elapsed", dur)
			} else {
				log.Warnw("file object opened, but context has been canceled", "path", p, "elapsed", dur, "err", err.Error())
			}

		default:
			log.Debugw("file object opened", "path", p, "elapsed", dur)
		}

		if err != nil && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, objstore.ErrNotRegularFile)) {
			err = fmt.Errorf("obj %s: %w", p, objstore.ErrObjectNotFound)
		}

		resCh <- readerResult{
			ReadCloser: r,
			Err:        err,
		}

	}()

	select {
	case <-ctx.Done():
		return readerResult{Err: fmt.Errorf("obj %s: %w", p, ctx.Err())}

	case res := <-resCh:
		return res
	}
}

func (s *Store) open(p string, r *readRange) (io.ReadCloser, error) {
	file, err := s.dir.Open(p)
	if err != nil {
		return nil, fmt.Errorf("obj %s: open: %w", p, err)
	}

	hold := false
	defer func() {
		if !hold {
			file.Close()
		}
	}()

	if s.cfg.Strict {
		stat, err := file.Stat()
		if err != nil {
			return nil, fmt.Errorf("obj %s: get stat: %w", p, err)
		}

		if !stat.Mode().Type().IsRegular() {
			file.Close()
			return nil, fmt.Errorf("obj %s: %w", p, objstore.ErrNotRegularFile)
		}
	}

	var reader io.ReadCloser = file
	if r != nil {
		seek, ok := file.(io.Seeker)
		if !ok {
			return nil, fmt.Errorf("obj %s: %w", p, objstore.ErrNotSeekable)
		}

		_, err = seek.Seek(r.Offset, io.SeekStart)
		if err != nil {
			return nil, fmt.Errorf("obj %s: seek to %d: %w", p, r.Offset, err)
		}

		reader = &limitedFile{
			Reader: io.LimitReader(file, r.Size),
			file:   file,
		}
	}

	hold = true
	return reader, nil
}

func (s *Store) Instance(context.Context) string { return s.cfg.Name }

func (s *Store) InstanceConfig(ctx context.Context) objstore.Config {
	return s.cfg
}

func (s *Store) InstanceInfo(ctx context.Context) (objstore.InstanceInfo, error) {
	usage, err := disk.UsageWithContext(ctx, s.cfg.Path)
	if err != nil {
		return objstore.InstanceInfo{}, fmt.Errorf("get disk usage: %w", err)
	}

	return objstore.InstanceInfo{
		Config:      s.cfg,
		Type:        usage.Fstype,
		Total:       usage.Total,
		Free:        usage.Free,
		Used:        usage.Used,
		UsedPercent: usage.UsedPercent,
	}, nil
}

func (s *Store) Get(ctx context.Context, p string) (io.ReadCloser, error) {
	res := s.openWithContext(ctx, p, nil)
	return res.ReadCloser, res.Err
}

func (s *Store) Del(ctx context.Context, p string) error {
	if s.cfg.ReadOnly {
		return objstore.ErrReadOnlyStore
	}

	fpath, err := s.getAbsPath(p)
	if err != nil {
		return err
	}

	err = os.Remove(fpath)
	if err != nil {
		return fmt.Errorf("del obj: %w", err)
	}

	return nil
}

func (s *Store) Stat(ctx context.Context, p string) (objstore.Stat, error) {
	resCh := make(chan statOrErr, 1)
	go func() {
		defer close(resCh)

		var res statOrErr

		finfo, err := os.Stat(s.FullPath(ctx, p))
		if err == nil {
			res.Stat.Size = finfo.Size()
		} else {
			res.Err = err
		}

		resCh <- res
	}()

	select {
	case <-ctx.Done():
		return objstore.Stat{}, ctx.Err()

	case res := <-resCh:
		return res.Stat, res.Err
	}
}

func (s *Store) getAbsPath(p string) (string, error) {
	fpath, err := filepath.Abs(filepath.Join(s.cfg.Path, p))
	if err != nil {
		return "", fmt.Errorf("obj %s: %w", p, objstore.ErrInvalidObjectPath)
	}

	if !strings.HasPrefix(fpath, s.cfg.Path) {
		return "", fmt.Errorf("obj %s: %w: outside of the dir", p, objstore.ErrInvalidObjectPath)
	}

	return fpath, nil
}

func (s *Store) Put(ctx context.Context, p string, r io.Reader) (int64, error) {
	if s.cfg.ReadOnly {
		return 0, objstore.ErrReadOnlyStore
	}

	fpath, err := s.getAbsPath(p)
	if err != nil {
		return 0, err
	}

	file, err := os.OpenFile(fpath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		return 0, fmt.Errorf("obj %s: create %w", p, err)
	}

	defer file.Close()

	return io.Copy(file, r)
}

func (s *Store) FullPath(ctx context.Context, sub string) string {
	return filepath.Join(s.cfg.Path, sub)
}

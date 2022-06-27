package objstore

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/plugable"
)

var _ Store = (*Wrapped)(nil)

type Wrapped struct {
	cfg   Config
	inner plugable.Plugable
}

func (w *Wrapped) Instance(context.Context) string {
	return w.cfg.Name
}

func (w *Wrapped) InstanceConfig(ctx context.Context) Config {
	return w.cfg
}

func (w *Wrapped) InstanceInfo(ctx context.Context) (InstanceInfo, error) {
	free, total, err := w.inner.Volume(ctx)
	if err != nil {
		return InstanceInfo{}, fmt.Errorf("get volume: %w", err)
	}

	var used uint64
	if total > free {
		used = total - free
	}

	usedPercent := 0.0
	// if used > 0, then total must not be zero
	if used > 0 {
		usedPercent = math.Ceil(float64(used)*10000/float64(total)) / 100
	}

	return InstanceInfo{
		Config:      w.cfg,
		Type:        w.inner.Type(),
		Total:       total,
		Free:        free,
		Used:        used,
		UsedPercent: usedPercent,
	}, nil
}

func (w *Wrapped) Get(ctx context.Context, p string) (io.ReadCloser, error) {
	r, err := w.inner.Get(ctx, p)
	return r, convertErrNotExist(err)
}

func (w *Wrapped) Del(ctx context.Context, p string) error {
	if w.cfg.ReadOnly {
		return ErrReadOnlyStore
	}

	return convertErrNotExist(w.inner.Del(ctx, p))
}

func (w *Wrapped) Stat(ctx context.Context, p string) (Stat, error) {
	stat, err := w.inner.Stat(ctx, p)
	if err != nil {
		return Stat{}, convertErrNotExist(err)
	}

	return Stat{
		Size: stat.Size(),
	}, nil
}

func (w *Wrapped) Put(ctx context.Context, p string, r io.Reader) (int64, error) {
	if w.cfg.ReadOnly {
		return 0, ErrReadOnlyStore
	}

	return w.inner.Put(ctx, p, r)
}

func (w *Wrapped) FullPath(ctx context.Context, p string) string {
	return w.inner.FullPath(ctx, p)
}

func convertErrNotExist(err error) error {
	if err == nil {
		return nil
	}

	if os.IsNotExist(err) {
		return ErrObjectNotFound
	}

	return err
}

package main

import (
	"context"
	"fmt"
	"time"

	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"
)

var (
	config       *Config = nil
	defaultStore Store   = nil
)

func OnInit(ctx context.Context, pluginsDir string, manifest *managerplugin.Manifest) (err error) {
	config, err = LoadConfig(ctx, pluginsDir, true)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	defaultStore, err = NewBadgerStore(config.DataDir)
	return err
}

func OnShutdown(ctx context.Context, manifest *managerplugin.Manifest) error {
	return defaultStore.Close()
}

type ConcurrentLimiterHandler interface {
	Acquire(ctx context.Context, name string, id string) (string, error)
	Release(ctx context.Context, slot, id string) (ok bool, err error)
	Extend(ctx context.Context, slot, id string) (ok bool, err error)
}

func Handler() (string, interface{}) {
	return "DAMOCLES_LIMITER", &concurrentLimiterHandler{
		limiter: NewConcurrentLimiter(config.Concurrent, time.Duration(30)*time.Second, defaultStore),
	}
}

type concurrentLimiterHandler struct {
	limiter *ConcurrentLimiter
}

func (h *concurrentLimiterHandler) Acquire(_ context.Context, name string, id string) (string, error) {
	return h.limiter.Acquire(name, id)
}

func (h *concurrentLimiterHandler) Release(_ context.Context, slot string, id string) (ok bool, err error) {
	err = h.limiter.Release(slot, id)
	// jsonrpc does not allow returning nil
	ok = err == nil
	return
}

func (h *concurrentLimiterHandler) Extend(_ context.Context, slot string, id string) (ok bool, err error) {
	return h.limiter.Extend(slot, id)
}

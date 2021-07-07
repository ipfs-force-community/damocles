package confmgr

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

type cfgItem struct {
	c      interface{}
	wlock  WLocker
	cancel context.CancelFunc
}

type localMgr struct {
	dir     string
	watcher *fsnotify.Watcher
	delay   time.Duration

	regmu sync.RWMutex
	reg   map[string]*cfgItem
}

func (lm *localMgr) Run(ctx context.Context) error {
	go lm.run(ctx)
	return nil
}

func (lm *localMgr) run(ctx context.Context) {
	log.Info("local conf mgr start")
	defer log.Info("local conf mgr stop")

	for {
		select {
		case <-ctx.Done():
			log.Warnf("context done")
			return

		case event, ok := <-lm.watcher.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Write != fsnotify.Write {
				continue
			}

			lm.regmu.RLock()
			item, has := lm.reg[event.Name]
			lm.regmu.RUnlock()
			if !has {
				log.Warnw("config item not found", "fname", event.Name)
				continue
			}

			lctx, lcancel := context.WithCancel(ctx)

			if item.cancel != nil {
				item.cancel()
			}

			item.cancel = lcancel

			go lm.loadModified(lctx, event.Name, item)

		case err, ok := <-lm.watcher.Errors:
			if !ok {
				log.Warnf("watcher error chan closed")
				return
			}

			log.Warnf("watcher error: %s", err)
		}
	}

}

func (lm *localMgr) Close(ctx context.Context) error {
	lm.watcher.Close()
	return nil
}

func (lm *localMgr) Load(ctx context.Context, key string, c interface{}) error {
	fname := filepath.Join(lm.dir, key)
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", fname, err)
	}

	return toml.Unmarshal(data, c)
}

func (lm *localMgr) Watch(ctx context.Context, key string, c interface{}, wlock WLocker) error {
	fname := filepath.Join(lm.dir, key)
	lm.regmu.Lock()
	defer lm.regmu.Unlock()

	if _, ok := lm.reg[fname]; ok {
		return fmt.Errorf("duplicate watching file %s", fname)
	}

	if err := lm.watcher.Add(fname); err != nil {
		return fmt.Errorf("add watcher for %s: %w", fname, err)
	}

	lm.reg[fname] = &cfgItem{
		c:      c,
		wlock:  wlock,
		cancel: nil,
	}

	return nil
}

func (lm *localMgr) loadModified(ctx context.Context, fname string, c *cfgItem) {
	ctx, cancel := context.WithTimeout(ctx, lm.delay)
	defer cancel()

	l := log.With("fname", fname)

	select {
	case <-ctx.Done():

	}

	cerr := ctx.Err()
	if cerr == context.Canceled {
		l.Debug("loading canceled")
		return
	}

	data, err := ioutil.ReadFile(fname)
	if err != nil {
		l.Errorf("failed to load data: %s", err)
		return
	}

	c.wlock.Lock()
	err = toml.Unmarshal(data, c.c)
	c.wlock.Unlock()

	if err != nil {
		l.Errorf("failed to unmarshal: %s", err)
	}
}

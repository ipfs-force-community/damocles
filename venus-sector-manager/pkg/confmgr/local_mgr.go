package confmgr

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
)

type cfgItem struct {
	c      interface{}
	crv    reflect.Value
	wlock  WLocker
	cancel context.CancelFunc
	newfn  func() interface{}
}

func NewLocal(dir string) (ConfigManager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("construct fsnotify watcher: %w", err)
	}

	return &localMgr{
		dir:     dir,
		watcher: watcher,
		delay:   10 * time.Second,
		reg:     map[string]*cfgItem{},
	}, nil
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

			if event.Op&fsnotify.Rename != fsnotify.Rename {
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

func (lm *localMgr) cfgpath(key string) string {
	return filepath.Join(lm.dir, fmt.Sprintf("%s.cfg", key))
}

func (lm *localMgr) SetDefault(ctx context.Context, key string, c interface{}) error {
	content, err := ConfigComment(c)
	if err != nil {
		return fmt.Errorf("marshal default content: %w", err)
	}

	fname := lm.cfgpath(key)
	return ioutil.WriteFile(fname, content, 0644)
}

func (lm *localMgr) Load(ctx context.Context, key string, c interface{}) error {
	fname := lm.cfgpath(key)
	data, err := ioutil.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", fname, err)
	}

	return toml.Unmarshal(data, c)
}

func (lm *localMgr) Watch(ctx context.Context, key string, c interface{}, wlock WLocker, newfn func() interface{}) error {

	maybe := newfn()
	valC := reflect.ValueOf(c)
	typC := valC.Type()
	typMaybe := reflect.TypeOf(maybe)

	if typC != typMaybe {
		return fmt.Errorf("config type not match, target=%s, newed=%s", typC, typMaybe)
	}

	if kind := typC.Kind(); kind != reflect.Ptr {
		return fmt.Errorf("config target should be pointer, got %s", kind)
	}

	fname := lm.cfgpath(key)

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
		crv:    valC,
		wlock:  wlock,
		cancel: nil,
		newfn:  newfn,
	}

	log.Infof("start to watch %s(%s)", key, fname)

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

	obj := c.newfn()
	err = toml.Unmarshal(data, obj)
	if err != nil {
		l.Errorf("failed to unmarshal data: %s", err)
		return
	}

	c.wlock.Lock()
	c.crv.Elem().Set(reflect.ValueOf(obj).Elem())
	c.wlock.Unlock()
	buf := bytes.Buffer{}
	encode := toml.NewEncoder(&buf)
	err = encode.Encode(obj)
	if err != nil {
		l.Errorf("failed to marshal obj: %s", err)
		return
	}

	l.Infof("%s loaded & updated, after updated cfg is %#v\n", fname, buf.String())
}

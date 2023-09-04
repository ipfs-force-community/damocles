package confmgr

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
)

type cfgItem struct {
	c      interface{}
	crv    reflect.Value
	wlock  WLocker
	cancel context.CancelFunc
	newfn  func() interface{}
}

func NewLocal(dir string) (ConfigManager, error) {
	return &localMgr{
		dir:   dir,
		delay: 10 * time.Second,
		reg:   map[string]*cfgItem{},
	}, nil
}

type localMgr struct {
	dir   string
	delay time.Duration

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

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)

	for {
		select {
		case <-ctx.Done():
			log.Warnf("context done")
			return

		case <-ch:
			lm.regmu.Lock()
			for i := range lm.reg {
				go lm.loadModified(ctx, i, lm.reg[i])
			}
			lm.regmu.Unlock()
		}

	}

}

func (lm *localMgr) Close(context.Context) error {
	return nil
}

func (lm *localMgr) cfgpath(key string) string {
	return filepath.Join(lm.dir, fmt.Sprintf("%s.cfg", key))
}

func (lm *localMgr) SetDefault(_ context.Context, key string, c interface{}) error {
	fname := lm.cfgpath(key)
	_, err := os.Stat(fname)
	if err == nil {
		return fmt.Errorf("%s already exits", fname)
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("stat file %s: %w", fname, err)
	}

	content, err := ConfigComment(c)
	if err != nil {
		return fmt.Errorf("marshal default content: %w", err)
	}

	return os.WriteFile(fname, content, 0644)
}

func (lm *localMgr) Load(_ context.Context, key string, c interface{}) error {
	fname := lm.cfgpath(key)
	data, err := os.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("failed to load %s: %w", fname, err)
	}

	return lm.unmarshal(data, c)
}

func (lm *localMgr) unmarshal(data []byte, obj interface{}) error {
	if un, ok := obj.(ConfigUnmarshaller); ok {
		return un.UnmarshalConfig(data)
	}

	return toml.Unmarshal(data, obj)
}

func (lm *localMgr) Watch(_ context.Context, key string, c interface{}, wlock WLocker, newfn func() interface{}) error {

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

	lm.reg[fname] = &cfgItem{
		c:      c,
		crv:    valC,
		wlock:  wlock,
		cancel: nil,
		newfn:  newfn,
	}

	log.Infof("will reload %s(%s) once receive reload sig", key, fname)

	return nil
}

func (lm *localMgr) loadModified(ctx context.Context, fname string, c *cfgItem) {
	ctx, cancel := context.WithTimeout(ctx, lm.delay)
	defer cancel()

	l := log.With("fname", fname)

	<-ctx.Done()

	cerr := ctx.Err()
	if cerr == context.Canceled {
		l.Debug("loading canceled")
		return
	}

	data, err := os.ReadFile(fname)
	if err != nil {
		l.Errorf("failed to load data: %s", err)
		return
	}

	obj := c.newfn()
	err = lm.unmarshal(data, obj)
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

	l.Infof("%s loaded & updated, after updated cfg is %s\n", fname, buf.String())
}

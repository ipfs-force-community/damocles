package confmgr

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("confmgr")

var (
	_ ConfigManager = (*localMgr)(nil)
)

type RLocker interface {
	sync.Locker
}

type WLocker interface {
	sync.Locker
}

type ConfigManager interface {
	Load(ctx context.Context, key string, c interface{}) error
	SetDefault(ctx context.Context, key string, c interface{}) error
	Watch(ctx context.Context, key string, c interface{}, wlock WLocker, newfn func() interface{}) error
	Run(ctx context.Context) error
	Close(ctx context.Context) error
}

func ConfigComment(t interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("# Default config:\n")
	e := toml.NewEncoder(buf)
	if err := e.Encode(t); err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	b := buf.Bytes()
	b = bytes.ReplaceAll(b, []byte("\n"), []byte("\n#"))
	b = bytes.ReplaceAll(b, []byte("#["), []byte("["))
	return b, nil
}

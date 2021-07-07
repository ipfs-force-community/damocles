package confmgr

import (
	"context"
	"sync"

	"github.com/dtynn/venus-cluster/venus-sealer/logging"
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
	Watch(ctx context.Context, key string, c interface{}, wlock WLocker) error
	Run(ctx context.Context) error
	Close(ctx context.Context) error
}

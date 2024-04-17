package confmgr

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var log = logging.New("confmgr")

var _ ConfigManager = (*localMgr)(nil)

type ConfigUnmarshaller interface {
	UnmarshalConfig([]byte) error
}

type CommentAll interface {
	CommentAllInExample()
}

type RLocker interface {
	sync.Locker
}

type WLocker interface {
	sync.Locker
}

type ConfigManager interface {
	Load(ctx context.Context, key string, c any) error
	SetDefault(ctx context.Context, key string, c any) error
	Watch(ctx context.Context, key string, c any, wlock WLocker, newfn func() any) error
	Run(ctx context.Context) error
	Close(ctx context.Context) error
}

func ConfigComment(t any) ([]byte, error) {
	buf := new(bytes.Buffer)
	_, _ = buf.WriteString("# Default config:\n")
	e := toml.NewEncoder(buf)
	e.Indent = ""
	if err := e.Encode(t); err != nil {
		return nil, fmt.Errorf("encoding config: %w", err)
	}
	b := buf.Bytes()
	b = bytes.ReplaceAll(b, []byte("\n"), []byte("\n#"))

	_, yes := t.(CommentAll)
	if !yes {
		b = bytes.ReplaceAll(b, []byte("#["), []byte("["))
	}

	return b, nil
}

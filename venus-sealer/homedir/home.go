package homedir

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
	"github.com/mitchellh/go-homedir"
)

func Open(path string) (*Home, error) {
	dir, err := homedir.Expand(path)
	if err != nil {
		return nil, fmt.Errorf("expand home dir: %w", err)
	}

	conf, err := confmgr.NewLocal(dir, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("construct local conf mgr: %w", err)
	}

	return &Home{
		dir:  dir,
		Conf: conf,
	}, nil
}

type Home struct {
	dir  string
	Conf confmgr.ConfigManager
}

func (h *Home) Dir() string {
	return h.dir
}

func (h *Home) Sub(elem ...string) string {
	return filepath.Join(h.dir, filepath.Join(elem...))
}

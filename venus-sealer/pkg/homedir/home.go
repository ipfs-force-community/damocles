package homedir

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mitchellh/go-homedir"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/confmgr"
)

func Open(path string) (*Home, error) {
	dir, err := homedir.Expand(path)
	if err != nil {
		return nil, fmt.Errorf("expand home dir: %w", err)
	}

	return &Home{
		dir: dir,
	}, nil
}

type Home struct {
	dir  string
	Conf confmgr.ConfigManager
}

func (h *Home) Init() error {
	if err := os.MkdirAll(h.dir, 0755); err != nil {
		return fmt.Errorf("mkdir for home: %w", err)
	}

	return nil
}

func (h *Home) Dir() string {
	return h.dir
}

func (h *Home) Sub(elem ...string) string {
	return filepath.Join(h.dir, filepath.Join(elem...))
}

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/filecoin-project/go-state-types/abi"
	plugin "github.com/ipfs-force-community/damocles/manager-plugin"
	pluginfilestore "github.com/ipfs-force-community/damocles/manager-plugin/filestore"
	"github.com/shirou/gopsutil/disk"
)

var _ pluginfilestore.Store = (*Store)(nil)

func OnInit(ctx context.Context, pluginsDir string, manifest *plugin.Manifest) error {
	return nil
}

func Open(cfg pluginfilestore.Config) (pluginfilestore.Store, error) { // nolint: deadcode
	dirPath, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, fmt.Errorf("abs path for %s: %w", cfg.Path, err)
	}

	stat, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("stat for %s: %w", dirPath, err)
	}

	if !stat.IsDir() {
		return nil, fmt.Errorf("%s is not a dir", dirPath)
	}

	cfg.Path = dirPath
	if cfg.Name == "" {
		cfg.Name = dirPath
	}

	return &Store{
		cfg: cfg,
	}, nil
}

type Store struct {
	cfg pluginfilestore.Config
}

func (s *Store) Type() string {
	return "builtin-filesotre"
}

func (*Store) Version() string {
	return "example"
}

func (s *Store) Instance(context.Context) string { return s.cfg.Name }

func (s *Store) InstanceConfig(_ context.Context) pluginfilestore.Config {
	return s.cfg
}

func (s *Store) InstanceInfo(ctx context.Context) (pluginfilestore.InstanceInfo, error) {
	usage, err := disk.UsageWithContext(ctx, s.cfg.Path)
	if err != nil {
		return pluginfilestore.InstanceInfo{}, fmt.Errorf("get disk usage: %w", err)
	}

	return pluginfilestore.InstanceInfo{
		Config:      s.cfg,
		Type:        usage.Fstype,
		Total:       usage.Total,
		Free:        usage.Free,
		Used:        usage.Used,
		UsedPercent: usage.UsedPercent,
	}, nil
}

func (s *Store) SubPath(ctx context.Context, pathType pluginfilestore.PathType, sectorID *pluginfilestore.SectorID, custom *string) (subPath string, err error) {
	if pathType == pluginfilestore.PathTypeCustom {
		if custom == nil {
			return "", fmt.Errorf("sectorID cannot be nil")
		}
		// just return custom, or return an error.
		// return nil, fmt.Errorf("PathType custom is not support");
		return *custom, nil
	}

	if sectorID == nil {
		return "", fmt.Errorf("sectorID cannot be nil")
	}

	sid := abi.SectorID{
		Miner:  abi.ActorID(sectorID.Miner),
		Number: abi.SectorNumber(sectorID.Number),
	}

	return filepath.Join(string(pathType), FormatSectorID(sid)), nil
}

const sectorIDFormat = "s-t0%d-%d"

func FormatSectorID(sid abi.SectorID) string {
	return fmt.Sprintf(sectorIDFormat, sid.Miner, sid.Number)
}

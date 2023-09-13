package builtin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/shirou/gopsutil/v3/disk"

	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/filestore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/ver"
)

var log = logging.New("filestore-builtin")

var _ filestore.Store = (*Store)(nil)

func New(cfg filestore.Config) (filestore.Store, error) {
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
	cfg filestore.Config
}

func (s *Store) Type() string {
	return "builtin-filesotre"
}

func (*Store) Version() string {
	return ver.VersionStr()
}

func (s *Store) Instance(context.Context) string { return s.cfg.Name }

func (s *Store) InstanceConfig(_ context.Context) filestore.Config {
	return s.cfg
}

func (s *Store) InstanceInfo(ctx context.Context) (filestore.InstanceInfo, error) {
	usage, err := disk.UsageWithContext(ctx, s.cfg.Path)
	if err != nil {
		return filestore.InstanceInfo{}, fmt.Errorf("get disk usage: %w", err)
	}

	return filestore.InstanceInfo{
		Config:      s.cfg,
		Type:        usage.Fstype,
		Total:       usage.Total,
		Free:        usage.Free,
		Used:        usage.Used,
		UsedPercent: usage.UsedPercent,
	}, nil
}

func (s *Store) SubPath(ctx context.Context, pathType filestore.PathType, sectorID *filestore.SectorID, custom *string) (subPath string, err error) {
	if pathType == filestore.PathTypeCustom {
		if custom == nil {
			return "", fmt.Errorf("sectorID cannot be nil")
		}
		return *custom, nil
	}

	if sectorID == nil {
		return "", fmt.Errorf("sectorID cannot be nil")
	}

	sid := abi.SectorID{
		Miner:  abi.ActorID(sectorID.Miner),
		Number: abi.SectorNumber(sectorID.Number),
	}

	switch pathType {
	case filestore.PathTypeSealed:
		subPath = util.SectorPath(util.SectorPathTypeSealed, sid)
	case filestore.PathTypeUpdate:
		subPath = util.SectorPath(util.SectorPathTypeUpdate, sid)
	case filestore.PathTypeCache:
		subPath = util.SectorPath(util.SectorPathTypeCache, sid)
	case filestore.PathTypeUpdateCache:
		subPath = util.SectorPath(util.SectorPathTypeUpdateCache, sid)
	default:
		return "", fmt.Errorf("unsupport path type: %s", pathType)
	}

	return subPath, nil
}

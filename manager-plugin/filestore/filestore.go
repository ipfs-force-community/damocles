package filestore

import (
	"context"
)

type PathType string

const (
	PathTypeSealed      PathType = "sealed"
	PathTypeUpdate      PathType = "update"
	PathTypeCache       PathType = "cache"
	PathTypeUpdateCache PathType = "update-cache"
	PathTypeCustom      PathType = "__custom__"
)

type Config struct {
	Name string
	Path string // In general, path is the mount point
	Meta map[string]string

	// Strict should never be used directly, use GetStrict() instead
	Strict *bool
	// ReadOnly should never be used directly, use GetReadOnly() instead
	ReadOnly *bool
	// Weight should never be used directly, use GetWeight() instead
	Weight *uint
}

func (c Config) GetStrict() bool {
	if c.Strict == nil {
		return false
	}
	return *c.Strict
}

func (c Config) GetReadOnly() bool {
	if c.ReadOnly == nil {
		return false
	}
	return *c.ReadOnly
}

func (c Config) GetWeight() uint {
	if c.Weight == nil {
		return 1
	}
	return *c.Weight
}

func DefaultConfig(path string, readonly bool) Config {
	one := uint(1)
	return Config{
		Path:     path,
		Meta:     map[string]string{},
		ReadOnly: &readonly,
		Weight:   &one,
	}
}

type InstanceInfo struct {
	Config      Config
	Type        string  // the type of this store, just for display
	Total       uint64  // the total capacity of this store, in bytes
	Free        uint64  // the free capacity of this store, in bytes
	Used        uint64  // the used capacity of this store, in bytes
	UsedPercent float64 // the percentage of used capacity for this store.
}

type SectorID struct {
	Miner  uint64
	Number uint64
}

type Store interface {
	// Type returns the type of this store, just for display.
	Type() string
	// Version returns the version of this store, just for display.
	Version() string
	Instance(context.Context) string
	// InstanceConfig returns the configuration of this store.
	InstanceConfig(ctx context.Context) Config
	// InstanceInfo returns the information of this store.
	InstanceInfo(ctx context.Context) (InstanceInfo, error)

	// SubPath returns the subpath of given params
	// if `pathType` is `PathTypeCustom` then `custom` is not nil otherwise `sectorID` is not nil
	//
	// Example:
	// ```go
	// assert(SubPath(context.TODO(), "cache", sid4040_1001, nil) == ("cache/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "sealed", sid4040_1001, nil) == ("sealed/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "update", sid4040_1001, nil) == ("update/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "update-cache", sid4040_1001, nil) == ("update-cache/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "custom", sid4040_1001, "path/to/anything") == ("path/to/anything", nil))
	// -- OR --
	// assert(SubPath(context.TODO(), "cache", sid4040_1001, nil) == ("myprefix/mycache/mysubdir/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "sealed", sid4040_1001, nil) == ("myprefix/mysealed/mysubdir/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "update", sid4040_1001, nil) == ("myprefix/myupdate/mysubdir/s-t04040-1001", nil))
	// assert(SubPath(context.TODO(), "update-cache", sid4040_1001, nil) == ("myprefix/myupdate-cache/mysubdir/s-t04040-1001", nil))
	// ...
	// ```
	SubPath(ctx context.Context, pathType PathType, sectorID *SectorID, custom *string) (subPath string, err error)
}

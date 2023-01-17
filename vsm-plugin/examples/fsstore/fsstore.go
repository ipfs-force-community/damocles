package main

import (
	"context"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
	vsmplugin "github.com/ipfs-force-community/venus-cluster/vsm-plugin"
	objstore "github.com/ipfs-force-community/venus-cluster/vsm-plugin/objstore"
)

func OnInit(ctx context.Context, manifest *vsmplugin.Manifest) error { return nil }

func Open(cfg objstore.Config) (objstore.Store, error) { // nolint: deadcode
	return filestore.Open(cfg, true)
}

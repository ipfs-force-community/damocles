package main

import (
	"context"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore/filestore"
	managerplugin "github.com/ipfs-force-community/damocles/manager-plugin"
	objstore "github.com/ipfs-force-community/damocles/manager-plugin/objstore"
)

func OnInit(ctx context.Context, pluginsDir string, manifest *managerplugin.Manifest) error {
	return nil
}

func Open(cfg objstore.Config) (objstore.Store, error) { // nolint: deadcode
	return filestore.Open(cfg, true)
}

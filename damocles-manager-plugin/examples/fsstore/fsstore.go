package main

import (
	"context"

	managerplugin "github.com/ipfs-force-community/damocles/damocles-manager-plugin"
	objstore "github.com/ipfs-force-community/damocles/damocles-manager-plugin/objstore"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/objstore/filestore"
)

func OnInit(ctx context.Context, pluginsDir string, manifest *managerplugin.Manifest) error {
	return nil
}

func Open(cfg objstore.Config) (objstore.Store, error) { // nolint: deadcode
	return filestore.Open(cfg, true)
}

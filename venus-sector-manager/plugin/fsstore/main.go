package main

import (
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/objstore/filestore"
)

var Constructor = filestore.Open // nolint: deadcode

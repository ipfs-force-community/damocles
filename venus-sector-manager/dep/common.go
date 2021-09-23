package dep

import (
	"github.com/dtynn/dix"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var log = logging.New("dep")

const (
	ignoredInvoke dix.Invoke = iota
	InjectSealerAPI
	InjectChainAPI
	InjectMessagerAPI
	StartPoSter
	InjectSealerClient
)

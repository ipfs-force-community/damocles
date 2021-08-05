package dep

import (
	"github.com/dtynn/dix"

	"github.com/dtynn/venus-cluster/venus-sealer/pkg/logging"
)

var log = logging.New("dep")

const (
	ignoredInvoke dix.Invoke = iota
	InjectSealerAPI
	InjectChainAPI
	InjectMessagerAPI
)

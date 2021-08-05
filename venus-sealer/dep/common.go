package dep

import (
	"github.com/dtynn/dix"
)

const (
	ignoredInvoke dix.Invoke = iota
	InjectSealerAPI
	InjectChainAPI
	InjectMessagerAPI
)

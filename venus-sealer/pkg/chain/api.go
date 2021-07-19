package chain

import (
	"github.com/filecoin-project/venus/app/submodule/apiface"
)

type API interface {
	apiface.IChainInfo
	apiface.ISyncer
	apiface.IActor
	apiface.IBlockStore
	apiface.IMinerState
}

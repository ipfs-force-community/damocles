package dep

import (
	"github.com/filecoin-project/go-address"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/market"
	"go.uber.org/fx"
)

type IMarketEvent = market.IMarketEvent

func buildMarketEvent(gctx GlobalContext, lc fx.Lifecycle, cfg *modules.SafeConfig) (IMarketEvent, error) {
	cfg.Lock()
	urls, commonToken, minerCfgs := cfg.Common.API.Gateway, cfg.Common.API.Token, cfg.Miners
	cfg.Unlock()

	miners := make([]address.Address, 0, len(minerCfgs))
	for _, mcfg := range minerCfgs {
		mAddr, err := address.NewIDAddress(uint64(mcfg.Actor))
		if err != nil {
			return nil, err
		}
		miners = append(miners, mAddr)
	}

	return market.NewMarketEvent(gctx, lc, urls, commonToken, miners)
}

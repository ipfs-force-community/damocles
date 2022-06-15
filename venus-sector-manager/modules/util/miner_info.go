package util

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/venus/venus-shared/types"
)

func IsController(mi types.MinerInfo, addr address.Address) bool {
	if addr == mi.Owner || addr == mi.Worker {
		return true
	}

	for _, caddr := range mi.ControlAddresses {
		if caddr == addr {
			return true
		}
	}

	return false
}

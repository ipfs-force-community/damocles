package util

import (
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
)

func SectorExtendedToNormal(extended builtin.ExtendedSectorInfo) builtin.SectorInfo {
	return builtin.SectorInfo{
		SealProof:    extended.SealProof,
		SectorNumber: extended.SectorNumber,
		SealedCID:    extended.SealedCID,
	}
}

func SectorOnChainInfoToExtended(sector *miner.SectorOnChainInfo) builtin.ExtendedSectorInfo {
	return builtin.ExtendedSectorInfo{
		SectorNumber: sector.SectorNumber,
		SealedCID:    sector.SealedCID,
		SealProof:    sector.SealProof,
		SectorKey:    sector.SectorKeyCID,
	}
}

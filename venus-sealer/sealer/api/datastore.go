package api

import (
	"context"

	"github.com/filecoin-project/go-state-types/abi"
)

type SectorsDatastore interface {
	GetSector(ctx context.Context, sectorID abi.SectorID) (Sector, error)
	PutSector(ctx context.Context, sector Sector) error
}

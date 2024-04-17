package piece

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	cborutil "github.com/filecoin-project/go-cbor-util"
	"github.com/filecoin-project/go-state-types/abi"
	miner13 "github.com/filecoin-project/go-state-types/builtin/v13/miner"
	verifreg13 "github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	"github.com/filecoin-project/go-state-types/builtin/v9/verifreg"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/market"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	chainapi "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
)

// ProcessPieces returns either:
// - a list of piece activation manifests
// - a list of deal IDs, if all non-filler pieces are deal-id pieces
func ProcessPieces(
	ctx context.Context,
	sector *core.SectorState,
	chain chainapi.API,
	lookupID core.LookupID,
) ([]miner13.PieceActivationManifest, []abi.DealID, error) {
	pams := make([]miner13.PieceActivationManifest, 0, len(sector.Pieces))
	dealIDs := make([]abi.DealID, 0, len(sector.Pieces))

	sectorPieces := sector.SectorPiece()
	var hasDDO bool
	for _, piece := range sectorPieces {
		if piece.HasDealInfo() && !piece.IsBuiltinMarket() {
			hasDDO = true
		}
	}

	for _, piece := range sectorPieces {
		if !piece.HasDealInfo() {
			continue
		}
		pieceInfo := piece.PieceInfo()
		if hasDDO && piece.IsBuiltinMarket() {
			// mixed ddo and builtinmarket
			alloc, err := chain.StateGetAllocationIdForPendingDeal(ctx, piece.DealID(), types.EmptyTSK)
			if err != nil {
				return nil, nil, fmt.Errorf("getting allocation for deal %d: %w", piece, err)
			}

			clid, err := lookupID.StateLookupID(ctx, piece.Client())
			if err != nil {
				return nil, nil, fmt.Errorf("getting client address for deal %d: %w", piece.DealID(), err)
			}

			clientID, err := address.IDFromAddress(clid)
			if err != nil {
				return nil, nil, fmt.Errorf("getting client address for deal %d: %w", piece.DealID(), err)
			}

			var vac *miner13.VerifiedAllocationKey
			if alloc != verifreg.NoAllocationID {
				vac = &miner13.VerifiedAllocationKey{
					Client: abi.ActorID(clientID),
					ID:     verifreg13.AllocationId(alloc),
				}
			}
			payload, err := cborutil.Dump(piece.DealID())
			if err != nil {
				return nil, nil, fmt.Errorf("serializing deal id: %w", err)
			}

			pams = append(pams, miner13.PieceActivationManifest{
				CID:                   pieceInfo.Cid,
				Size:                  pieceInfo.Size,
				VerifiedAllocationKey: vac,
				Notify: []miner13.DataActivationNotification{
					{
						Address: market.Address,
						Payload: payload,
					},
				},
			})
			continue
		}

		if piece.IsBuiltinMarket() {
			dealIDs = append(dealIDs, piece.DealID())
			continue
		}

		// ddo
		clid, err := lookupID.StateLookupID(ctx, piece.Client())
		if err != nil {
			return nil, nil, fmt.Errorf("getting client address for deal %d: %w", piece.DealID(), err)
		}

		clientID, err := address.IDFromAddress(clid)
		if err != nil {
			return nil, nil, fmt.Errorf("getting client address for deal %d: %w", piece.DealID(), err)
		}

		pams = append(pams, miner13.PieceActivationManifest{
			CID:  piece.PieceInfo().Cid,
			Size: piece.PieceInfo().Size,
			VerifiedAllocationKey: &miner13.VerifiedAllocationKey{
				Client: abi.ActorID(clientID),
				ID:     verifreg13.AllocationId(piece.AllocationID()),
			},
			Notify: nil,
		})
	}

	return pams, dealIDs, nil
}

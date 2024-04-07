package commitmgr

import (
	"bytes"
	"context"
	"fmt"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-commp-utils/zerocomm"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"

	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
)

type ErrAPI struct{ error }

type ErrInvalidDeals struct{ error }
type ErrInvalidPiece struct{ error }
type ErrExpiredDeals struct{ error }

type ErrBadCommD struct{ error }
type ErrExpiredTicket struct{ error }
type ErrBadTicket struct{ error }
type ErrPrecommitOnChain struct{ error }
type ErrSectorNumberAllocated struct{ error }
type ErrMarshalAddr struct{ error }
type ErrBadSeed struct{ error }
type ErrInvalidProof struct{ error }
type ErrNoPrecommit struct{ error }
type ErrCommitWaitFailed struct{ error }

// checkPrecommit checks that data commitment generated in the sealing process
// matches pieces, and that the seal ticket isn't expired
func checkPrecommit(ctx context.Context, maddr address.Address, si *core.SectorState, api SealingAPI) (err error) {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrAPI{fmt.Errorf("get chain head failed %w", err)}
	}

	if err := checkPieces(ctx, maddr, si, api); err != nil {
		return err
	}

	if si.HasData() {
		commD, err := computeUnsealedCIDFromPieces(si)
		if err != nil {
			return &ErrAPI{fmt.Errorf("compute unsealed cid from pieces: %w", err)}
		}

		if si.Pre == nil {
			return &ErrBadCommD{fmt.Errorf("no pre commit on chain info available")}
		}

		if !commD.Equals(si.Pre.CommD) {
			return &ErrBadCommD{fmt.Errorf("on chain CommD differs: %s != %s(chain)", si.Pre.CommD, commD)}
		}
	}

	// never commit P2 message before, check ticket expiration
	ticketEarliest := height - policy.MaxPreCommitRandomnessLookback

	if si.Ticket.Epoch < ticketEarliest {
		return &ErrExpiredTicket{fmt.Errorf("ticket expired: seal height: %d, head: %d", si.Ticket.Epoch+policy.SealRandomnessLookback, height)}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.ID.Number, tok)
	if err != nil {
		if err == ErrSectorAllocated {
			return &ErrSectorNumberAllocated{err}
		}
		return &ErrAPI{fmt.Errorf("getting precommit info: %w", err)}
	}

	if pci != nil {
		if pci.Info.SealRandEpoch != si.Ticket.Epoch {
			return &ErrBadTicket{fmt.Errorf("bad ticket epoch: %d != %d", pci.Info.SealRandEpoch, si.Ticket.Epoch)}
		}
		return &ErrPrecommitOnChain{fmt.Errorf("precommit already on chain")}
	}

	return nil
}

func checkPieces(ctx context.Context, maddr address.Address, sector *core.SectorState, api SealingAPI) error {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrAPI{fmt.Errorf("getting chain head: %w", err)}
	}
	numDeals := len(sector.Pieces)
	if numDeals == 0 {
		numDeals = len(sector.LegacyPieces)
	}

	checkZeroPiece := func(pieceIdx int, piece abi.PieceInfo) error {
		exp := zerocomm.ZeroPieceCommitment(piece.Size.Unpadded())
		if !piece.PieceCID.Equals(exp) {
			return &ErrInvalidPiece{fmt.Errorf("sector %d piece %d had non-zero PieceCID %+v", sector.ID, pieceIdx, piece.PieceCID)}
		}
		return nil
	}

	checkBuiltinMarketPiece := func(pieceIdx int, dealID abi.DealID, piece abi.PieceInfo) error {
		proposal, err := api.StateMarketStorageDealProposal(ctx, dealID, tok)
		if err != nil {
			return &ErrAPI{fmt.Errorf("getting deal %d for piece %d: %w", dealID, pieceIdx, err)}
		}

		if proposal.Provider != maddr {
			return &ErrInvalidDeals{fmt.Errorf("piece %d (of %v) of sector %d refers deal %d with wrong provider: %s != %s", pieceIdx, numDeals, sector.ID, dealID, proposal.Provider, maddr)}
		}

		if proposal.PieceCID != piece.PieceCID {
			return &ErrInvalidDeals{fmt.Errorf("piece %d (of %v) of sector %d refers deal %d with wrong PieceCID: %x != %x", pieceIdx, numDeals, sector.ID, dealID, piece.PieceCID, proposal.PieceCID)}
		}

		if piece.Size != proposal.PieceSize {
			return &ErrInvalidDeals{fmt.Errorf("piece %d (of %v) of sector %d refers deal %d with different size: %d != %d", pieceIdx, numDeals, sector.ID, dealID, piece.Size, proposal.PieceSize)}
		}

		if height >= proposal.StartEpoch {
			return &ErrExpiredDeals{fmt.Errorf("piece %d (of %v) of sector %d refers expired deal %d - should start at %d, head %d", pieceIdx, numDeals, sector.ID, dealID, proposal.StartEpoch, height)}
		}
		return nil
	}

	checkDDOPiece := func(pieceIdx int, allocationID types.AllocationId, piece abi.PieceInfo) error {
		// try to get allocation to see if that still works
		all, err := api.StateGetAllocation(ctx, maddr, allocationID, tok)
		if err != nil {
			return fmt.Errorf("getting deal %d allocation: %w", allocationID, err)
		}
		if all != nil {
			mid, err := address.IDFromAddress(maddr)
			if err != nil {
				return fmt.Errorf("getting miner id: %w", err)
			}

			if all.Provider != abi.ActorID(mid) {
				return fmt.Errorf("allocation provider doesn't match miner")
			}

			if height >= all.Expiration {
				return &ErrExpiredDeals{fmt.Errorf("piece allocation %d (of %d) of sector %d refers expired deal %d - should start at %d, head %d", pieceIdx, numDeals, sector.ID, allocationID, all.Expiration, height)}
			}

			if all.Size < piece.Size {
				return &ErrInvalidDeals{fmt.Errorf("piece allocation %d (of %d) of sector %d refers deal %d with different size: %d != %d", pieceIdx, numDeals, sector.ID, allocationID, piece.Size, all.Size)}
			}
		}
		return nil
	}

	for i, deal := range sector.LegacyPieces {
		pieceInfo := abi.PieceInfo{
			Size:     deal.Piece.Size,
			PieceCID: deal.Piece.Cid,
		}
		if deal.ID == 0 {
			if err := checkZeroPiece(i, pieceInfo); err != nil {
				return err
			}
		} else {
			if err := checkBuiltinMarketPiece(i, deal.ID, pieceInfo); err != nil {
				return err
			}
		}
	}

	for i, piece := range sector.Pieces {
		if !piece.HasDealInfo() {
			if err := checkZeroPiece(i, piece.Piece); err != nil {
				return err
			}
		} else if piece.DealInfo.IsBuiltinMarket {
			if err := checkBuiltinMarketPiece(i, piece.DealInfo.DealID, piece.Piece); err != nil {
				return err
			}
		} else {
			if err := checkDDOPiece(i, piece.DealInfo.AllocationID, piece.Piece); err != nil {
				return nil
			}
		}
	}

	return nil
}

func checkCommit(ctx context.Context, si *core.SectorState, proof []byte, tok core.TipSetToken, maddr address.Address, verif core.Verifier, api SealingAPI) (err error) {
	if si.Seed == nil {
		return &ErrBadSeed{fmt.Errorf("seed epoch was not set")}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.ID.Number, tok)
	if err == ErrSectorAllocated {
		return &ErrSectorNumberAllocated{err}
	}
	if err != nil {
		return &ErrAPI{fmt.Errorf("get sector precommit info failed")}
	}

	if pci == nil {
		return &ErrNoPrecommit{fmt.Errorf("precommit info not found on-chain")}
	}

	seedEpoch := pci.PreCommitEpoch + policy.GetPreCommitChallengeDelay()

	if seedEpoch != si.Seed.Epoch {
		return &ErrBadSeed{fmt.Errorf("seed epoch doesn't match on chain info: %d(chain) != %d", seedEpoch, si.Seed.Epoch)}
	}

	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return &ErrMarshalAddr{err}
	}

	seed, err := api.GetSeed(ctx, types.EmptyTSK, seedEpoch, si.ID.Miner)
	if err != nil {
		return &ErrAPI{fmt.Errorf("failed to get randomness for computing seal proof: %w", err)}
	}

	if !bytes.Equal(seed.Seed, si.Seed.Seed) {
		return &ErrBadSeed{fmt.Errorf("seed has changed")}
	}

	ok, err := verif.VerifySeal(ctx, core.SealVerifyInfo{
		SectorID:              si.ID,
		SealedCID:             pci.Info.SealedCID,
		SealProof:             pci.Info.SealProof,
		Proof:                 proof,
		Randomness:            abi.SealRandomness(si.Ticket.Ticket),
		InteractiveRandomness: abi.InteractiveSealRandomness(si.Seed.Seed),
		UnsealedCID:           si.Pre.CommD,
	})
	if err != nil {
		return &ErrInvalidProof{fmt.Errorf("verify seal: %w", err)}
	}
	if !ok {
		return &ErrInvalidProof{fmt.Errorf("invalid proof (compute error?)")}
	}

	return checkPieces(ctx, maddr, si, api)
}

func computeUnsealedCIDFromPieces(sector *core.SectorState) (cid.Cid, error) {
	return ffi.GenerateUnsealedCID(sector.SectorType, lo.Map(sector.SectorPiece(), func(p core.SectorPiece, i int) abi.PieceInfo {
		pi := p.PieceInfo()
		return abi.PieceInfo{
			Size:     pi.Size,
			PieceCID: pi.Cid,
		}
	}))
}

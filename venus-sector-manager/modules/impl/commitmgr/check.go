package commitmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"
	"github.com/filecoin-project/venus/pkg/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/api"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
)

type ErrApi struct{ error }

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
//  matches pieces, and that the seal ticket isn't expired
func checkPrecommit(ctx context.Context, maddr address.Address, si api.SectorState, api SealingAPI) (err error) {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return ErrApi{fmt.Errorf("get chain head failed %w", err)}
	}

	if err := checkPieces(ctx, maddr, si, api); err != nil {
		return err
	}

	commD, err := api.StateComputeDataCommitment(ctx, maddr, si.SectorType, si.DealIDs(), tok)
	if err != nil {
		return &ErrApi{fmt.Errorf("calling StateComputeDataCommitment: %w", err)}
	}

	if si.Pre == nil {
		return &ErrBadCommD{fmt.Errorf("no pre commit on chain info available")}
	}

	if !commD.Equals(si.Pre.CommD) {
		return &ErrBadCommD{fmt.Errorf("on chain CommD differs: %s != %s ", si.Pre.CommD, commD)}
	}

	ticketEarliest := height - policy.MaxPreCommitRandomnessLookback

	if si.Ticket.Epoch < ticketEarliest {
		return &ErrExpiredTicket{fmt.Errorf("ticket expired: seal height: %d, head: %d", si.Ticket.Epoch+policy.SealRandomnessLookback, height)}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.ID.Number, tok)
	if err != nil {
		if err == ErrSectorAllocated {
			return &ErrSectorNumberAllocated{err}
		}
		return &ErrApi{fmt.Errorf("getting precommit info: %w", err)}
	}

	if pci != nil {
		if pci.Info.SealRandEpoch != si.Ticket.Epoch {
			return &ErrBadTicket{fmt.Errorf("bad ticket epoch: %d != %d", pci.Info.SealRandEpoch, si.Ticket.Epoch)}
		}
		return &ErrPrecommitOnChain{fmt.Errorf("precommit already on chain")}
	}

	return nil
}

func checkPieces(ctx context.Context, maddr address.Address, si api.SectorState, api SealingAPI) error {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{fmt.Errorf("getting chain head: %w", err)}
	}

	for i, p := range si.Deals {
		// if no deal is associated with the piece, ensure that we added it as
		// filler (i.e. ensure that it has a zero PieceCID)

		proposal, err := api.StateMarketStorageDealProposal(ctx, p.ID, tok)
		if err != nil {
			return &ErrApi{fmt.Errorf("getting deal %d for piece %d: %w", p.ID, i, err)}
		}

		if proposal.Provider != maddr {
			return &ErrInvalidDeals{fmt.Errorf("piece %d (of %v) of sector %d refers deal %d with wrong provider: %s != %s", i, len(si.Deals), si.ID, p.ID, proposal.Provider, maddr)}
		}

		if proposal.PieceCID != p.Piece.PieceCID {
			return &ErrInvalidDeals{fmt.Errorf("piece %d (of %v) of sector %d refers deal %d with wrong PieceCID: %x != %x", i, len(si.Deals), si.ID, p.ID, p.Piece.PieceCID, proposal.PieceCID)}
		}

		if p.Piece.Size != proposal.PieceSize {
			return &ErrInvalidDeals{fmt.Errorf("piece %d (of %v) of sector %d refers deal %d with different size: %d != %d", i, len(si.Deals), si.ID, p.ID, p.Piece.Size, proposal.PieceSize)}
		}

		if height >= proposal.StartEpoch {
			return &ErrExpiredDeals{fmt.Errorf("piece %d (of %v) of sector %d refers expired deal %d - should start at %d, head %d", i, len(si.Deals), si.ID, p.ID, proposal.StartEpoch, height)}
		}
	}

	return nil
}

func checkCommit(ctx context.Context, si api.SectorState, proof []byte, tok api.TipSetToken, maddr address.Address, verif api.Verifier, api SealingAPI) (err error) {
	if si.Seed == nil {
		return &ErrBadSeed{fmt.Errorf("seed epoch was not set")}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.ID.Number, tok)
	if err == ErrSectorAllocated {
		return &ErrSectorNumberAllocated{err}
	}
	if err != nil {
		return &ErrApi{fmt.Errorf("get sector precommit info failed")}
	}

	if pci == nil {
		return &ErrNoPrecommit{fmt.Errorf("precommit info not found on-chain")}
	}

	seedEpoch := pci.PreCommitEpoch + policy.NetParams.Network.PreCommitChallengeDelay

	if seedEpoch != si.Seed.Epoch {
		return &ErrBadSeed{fmt.Errorf("seed epoch doesn't match on chain info: %d != %d", seedEpoch, si.Seed.Epoch)}
	}

	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return &ErrMarshalAddr{err}
	}

	seed, err := api.GetSeed(ctx, types.EmptyTSK, seedEpoch, si.ID.Miner)
	if err != nil {
		return &ErrApi{fmt.Errorf("failed to get randomness for computing seal proof: %w", err)}
	}

	if !bytes.Equal(seed.Seed, si.Seed.Seed) {
		return &ErrBadSeed{fmt.Errorf("seed has changed")}
	}

	ok, err := verif.VerifySeal(ctx, proof2.SealVerifyInfo{
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

	if err := checkPieces(ctx, maddr, si, api); err != nil {
		return err
	}

	return nil
}

package commitmgr

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	proof2 "github.com/filecoin-project/specs-actors/v2/actors/runtime/proof"
	"github.com/filecoin-project/venus/pkg/specactors/policy"
	"golang.org/x/xerrors"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
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

type ErrBadSeed struct{ error }
type ErrInvalidProof struct{ error }
type ErrNoPrecommit struct{ error }
type ErrCommitWaitFailed struct{ error }

// checkPrecommit checks that data commitment generated in the sealing process
//  matches pieces, and that the seal ticket isn't expired
func checkPrecommit(ctx context.Context, maddr address.Address, si api.Sector, api SealingAPI) (err error) {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return err
	}

	if err := checkPieces(ctx, maddr, si, api); err != nil {
		return err
	}

	commD, err := api.StateComputeDataCommitment(ctx, maddr, si.SectorType, si.DealIDs(), tok)
	if err != nil {
		return &ErrApi{xerrors.Errorf("calling StateComputeDataCommitment: %w", err)}
	}

	if si.CommD == nil || !commD.Equals(*si.CommD) {
		return &ErrBadCommD{xerrors.Errorf("on chain CommD differs from sector: %s != %s", commD, si.CommD)}
	}

	ticketEarliest := height - policy.MaxPreCommitRandomnessLookback

	if si.TicketEpoch < ticketEarliest {
		return &ErrExpiredTicket{xerrors.Errorf("ticket expired: seal height: %d, head: %d", si.TicketEpoch+policy.SealRandomnessLookback, height)}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.SectorID.Number, tok)
	if err != nil {
		if err == ErrSectorAllocated {
			return &ErrSectorNumberAllocated{err}
		}
		return &ErrApi{xerrors.Errorf("getting precommit info: %w", err)}
	}

	if pci != nil {
		if pci.Info.SealRandEpoch != si.TicketEpoch {
			return &ErrBadTicket{xerrors.Errorf("bad ticket epoch: %d != %d", pci.Info.SealRandEpoch, si.TicketEpoch)}
		}
		return &ErrPrecommitOnChain{xerrors.Errorf("precommit already on chain")}
	}

	return nil
}

func checkPieces(ctx context.Context, maddr address.Address, si api.Sector, api SealingAPI) error {
	tok, height, err := api.ChainHead(ctx)
	if err != nil {
		return &ErrApi{xerrors.Errorf("getting chain head: %w", err)}
	}

	for i, p := range si.Deals {
		// if no deal is associated with the piece, ensure that we added it as
		// filler (i.e. ensure that it has a zero PieceCID)

		proposal, err := api.StateMarketStorageDealProposal(ctx, p.ID, tok)
		if err != nil {
			return &ErrInvalidDeals{xerrors.Errorf("getting deal %d for piece %d: %w", p, i, err)}
		}

		if proposal.Provider != maddr {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (of %v) of sector %d refers deal %d with wrong provider: %s != %s", i, len(si.Deals), si.SectorID, p.ID, proposal.Provider, maddr)}
		}

		if proposal.PieceCID != p.Piece.PieceCID {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (of %v) of sector %d refers deal %d with wrong PieceCID: %x != %x", i, len(si.Deals), si.SectorID, p.ID, p.Piece.PieceCID, proposal.PieceCID)}
		}

		if p.Piece.Size != proposal.PieceSize {
			return &ErrInvalidDeals{xerrors.Errorf("piece %d (of %v) of sector %d refers deal %d with different size: %d != %d", i, len(si.Deals), si.SectorID, p.ID, p.Piece.Size, proposal.PieceSize)}
		}

		if height >= proposal.StartEpoch {
			return &ErrExpiredDeals{xerrors.Errorf("piece %d (of %v) of sector %d refers expired deal %d - should start at %d, head %d", i, len(si.Deals), si.SectorID, p.ID, proposal.StartEpoch, height)}
		}
	}

	return nil
}

func checkCommit(ctx context.Context, si api.Sector, proof []byte, tok api.TipSetToken, maddr address.Address, verif api.Verifier, api SealingAPI) (err error) {
	if si.SeedEpoch == 0 {
		return &ErrBadSeed{xerrors.Errorf("seed epoch was not set")}
	}

	pci, err := api.StateSectorPreCommitInfo(ctx, maddr, si.SectorID.Number, tok)
	if err == ErrSectorAllocated {
		// not much more we can check here, basically try to wait for commit,
		// and hope that this will work

		if si.CommitCid != nil {
			return &ErrCommitWaitFailed{err}
		}

		return err
	}
	if err != nil {
		return xerrors.Errorf("getting precommit info: %w", err)
	}

	if pci == nil {
		return &ErrNoPrecommit{xerrors.Errorf("precommit info not found on-chain")}
	}

	if pci.PreCommitEpoch+policy.GetPreCommitChallengeDelay() != si.SeedEpoch {
		return &ErrBadSeed{xerrors.Errorf("seed epoch doesn't match on chain info: %d != %d", pci.PreCommitEpoch+policy.GetPreCommitChallengeDelay(), si.SeedEpoch)}
	}

	buf := new(bytes.Buffer)
	if err := maddr.MarshalCBOR(buf); err != nil {
		return err
	}

	seed, err := api.ChainGetRandomnessFromBeacon(ctx, tok, crypto.DomainSeparationTag_InteractiveSealChallengeSeed, si.SeedEpoch, buf.Bytes())
	if err != nil {
		return &ErrApi{xerrors.Errorf("failed to get randomness for computing seal proof: %w", err)}
	}

	if string(seed) != string(si.SeedValue) {
		return &ErrBadSeed{xerrors.Errorf("seed has changed")}
	}

	if *si.CommR != pci.Info.SealedCID {
		log.Warn("on-chain sealed CID doesn't match!")
	}

	ok, err := verif.VerifySeal(proof2.SealVerifyInfo{
		SectorID:              si.SectorID,
		SealedCID:             pci.Info.SealedCID,
		SealProof:             pci.Info.SealProof,
		Proof:                 proof,
		Randomness:            si.TicketValue,
		InteractiveRandomness: si.SeedValue,
		UnsealedCID:           *si.CommD,
	})
	if err != nil {
		return &ErrInvalidProof{xerrors.Errorf("verify seal: %w", err)}
	}
	if !ok {
		return &ErrInvalidProof{xerrors.New("invalid proof (compute error?)")}
	}

	if err := checkPieces(ctx, maddr, si, api); err != nil {
		return err
	}

	return nil
}

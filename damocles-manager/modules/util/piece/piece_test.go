package piece

import (
	"context"
	"testing"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/verifreg"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/require"

	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	chainapi "github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
)

// stubChain answers the chain calls ProcessPieces can make. The embedded nil
// interface makes every other FullNode method panic rather than return a zero
// value the test would silently accept.
type stubChain struct {
	chainapi.API

	nv              network.Version
	allocationID    verifreg.AllocationId
	allocationCalls int
}

func (s *stubChain) StateNetworkVersion(context.Context, types.TipSetKey) (network.Version, error) {
	return s.nv, nil
}

func (s *stubChain) StateGetAllocationIdForPendingDeal(
	context.Context,
	abi.DealID,
	types.TipSetKey,
) (verifreg.AllocationId, error) {
	s.allocationCalls++
	return s.allocationID, nil
}

type stubLookup struct {
	id    address.Address
	calls int
}

func (s *stubLookup) StateLookupID(context.Context, address.Address) (address.Address, error) {
	s.calls++
	return s.id, nil
}

func builtinMarketSector(dealID abi.DealID, pieceCID cid.Cid, size abi.PaddedPieceSize) *core.SectorState {
	return &core.SectorState{
		LegacyPieces: core.Deals{{
			ID:       dealID,
			Piece:    core.PieceInfo{Size: size, Cid: pieceCID},
			Proposal: &core.DealProposal{Client: address.Undef},
		}},
	}
}

// FIP-0118 grants every sector full quality-adjusted power, which the PAM
// expresses as a fully verified sector. nv29 therefore must not ask the chain
// for a per-deal allocation: StateGetAllocationIdForPendingDeal is unsupported
// from actors v19 and fails the whole batch.
func TestProcessPiecesBuiltinMarketAllocationLookup(t *testing.T) {
	const dealID = abi.DealID(42)
	const allocationID = verifreg.AllocationId(7)
	const clientActorID = abi.ActorID(1000)

	pieceCID := cid.MustParse("bafkqaaa")
	const pieceSize = abi.PaddedPieceSize(32 << 30)

	cases := []struct {
		name             string
		nv               network.Version
		wantAllocLookups int
		wantVerifiedKey  bool
	}{
		{
			name:             "before nv29 the verified allocation is resolved",
			nv:               network.Version29 - 1,
			wantAllocLookups: 1,
			wantVerifiedKey:  true,
		},
		{
			name:             "nv29 skips the unsupported allocation lookup",
			nv:               network.Version29,
			wantAllocLookups: 0,
			wantVerifiedKey:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clientAddr, err := address.NewIDAddress(uint64(clientActorID))
			require.NoError(t, err)

			chain := &stubChain{nv: c.nv, allocationID: allocationID}
			lookup := &stubLookup{id: clientAddr}

			pams, dealIDs, err := ProcessPieces(
				context.Background(),
				builtinMarketSector(dealID, pieceCID, pieceSize),
				chain,
				lookup,
			)
			require.NoError(t, err)

			require.Equal(t, c.wantAllocLookups, chain.allocationCalls)
			require.Empty(t, dealIDs)
			require.Len(t, pams, 1)
			require.Equal(t, pieceCID, pams[0].CID)
			require.Equal(t, pieceSize, pams[0].Size)

			if !c.wantVerifiedKey {
				require.Nil(t, pams[0].VerifiedAllocationKey)
				require.Zero(t, lookup.calls)
				return
			}

			require.NotNil(t, pams[0].VerifiedAllocationKey)
			require.Equal(t, clientActorID, pams[0].VerifiedAllocationKey.Client)
			require.Equal(t, uint64(allocationID), uint64(pams[0].VerifiedAllocationKey.ID))
			require.Equal(t, 1, lookup.calls)
		})
	}
}

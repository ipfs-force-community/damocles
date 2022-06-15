package commitmgr

import (
	"context"
	"errors"

	"github.com/ipfs/go-cid"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	stminer "github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"

	"github.com/filecoin-project/venus/venus-shared/actors/builtin/market"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin/miner"
	"github.com/filecoin-project/venus/venus-shared/types"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
)

var ErrSectorAllocated = errors.New("sectorNumber is allocated, but PreCommit info wasn't found on chain")

type SealingAPI interface {
	StateComputeDataCommitment(ctx context.Context, maddr address.Address, sectorType abi.RegisteredSealProof, deals []abi.DealID, tok core.TipSetToken) (cid.Cid, error)

	// Can return ErrSectorAllocated in case precommit info wasn't found, but the sector number is marked as allocated
	StateSectorPreCommitInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok core.TipSetToken) (*stminer.SectorPreCommitOnChainInfo, error)
	StateSectorGetInfo(ctx context.Context, maddr address.Address, sectorNumber abi.SectorNumber, tok core.TipSetToken) (*miner.SectorOnChainInfo, error)
	StateMinerSectorSize(context.Context, address.Address, core.TipSetToken) (abi.SectorSize, error)
	StateMinerPreCommitDepositForPower(context.Context, address.Address, stminer.SectorPreCommitInfo, core.TipSetToken) (big.Int, error)
	StateMinerInitialPledgeCollateral(context.Context, address.Address, stminer.SectorPreCommitInfo, core.TipSetToken) (big.Int, error)
	StateMarketStorageDealProposal(context.Context, abi.DealID, core.TipSetToken) (market.DealProposal, error)
	StateMinerInfo(context.Context, address.Address, core.TipSetToken) (types.MinerInfo, error)
	StateMinerSectorAllocated(context.Context, address.Address, abi.SectorNumber, core.TipSetToken) (bool, error)
	StateNetworkVersion(ctx context.Context, tok core.TipSetToken) (network.Version, error)
	ChainHead(ctx context.Context) (core.TipSetToken, abi.ChainEpoch, error)
	ChainBaseFee(ctx context.Context, tok core.TipSetToken) (abi.TokenAmount, error)

	StateSectorPartition(context.Context, address.Address, abi.SectorNumber, core.TipSetToken) (*miner.SectorLocation, error)
	StateMinerPartitions(context.Context, address.Address, uint64, core.TipSetToken) ([]chain.Partition, error)
	StateMinerProvingDeadline(context.Context, address.Address, core.TipSetToken) (*dline.Info, error)

	GetSeed(context.Context, types.TipSetKey, abi.ChainEpoch, abi.ActorID) (core.Seed, error)
}

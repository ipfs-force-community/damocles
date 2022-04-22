package core

import (
	market7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/market"
	miner7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/miner"
	power7 "github.com/filecoin-project/specs-actors/v7/actors/builtin/power"
	proof7 "github.com/filecoin-project/specs-actors/v7/actors/runtime/proof"
)

type (
	ComputeDataCommitmentParams = market7.ComputeDataCommitmentParams
	ComputeDataCommitmentReturn = market7.ComputeDataCommitmentReturn
	SectorDataSpec              = market7.SectorDataSpec

	ChangeWorkerAddressParams  = miner7.ChangeWorkerAddressParams
	CompactSectorNumbersParams = miner7.CompactSectorNumbersParams
	PreCommitSectorBatchParams = miner7.PreCommitSectorBatchParams
	WithdrawBalanceParams      = miner7.WithdrawBalanceParams

	CreateMinerParams = power7.CreateMinerParams
	CreateMinerReturn = power7.CreateMinerReturn

	AggregateSealVerifyInfo          = proof7.AggregateSealVerifyInfo
	AggregateSealVerifyProofAndInfos = proof7.AggregateSealVerifyProofAndInfos
	SealVerifyInfo                   = proof7.SealVerifyInfo
	WindowPoStVerifyInfo             = proof7.WindowPoStVerifyInfo
)

const MinAggregatedSectors = miner7.MinAggregatedSectors

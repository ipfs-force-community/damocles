package core

import (
	market8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/market"
	miner8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/miner"
	power8 "github.com/filecoin-project/specs-actors/v8/actors/builtin/power"
	proof8 "github.com/filecoin-project/specs-actors/v8/actors/runtime/proof"
)

type (
	ComputeDataCommitmentParams = market8.ComputeDataCommitmentParams
	ComputeDataCommitmentReturn = market8.ComputeDataCommitmentReturn
	SectorDataSpec              = market8.SectorDataSpec

	ChangeWorkerAddressParams    = miner8.ChangeWorkerAddressParams
	CompactSectorNumbersParams   = miner8.CompactSectorNumbersParams
	ExpirationExtension          = miner8.ExpirationExtension
	ExtendSectorExpirationParams = miner8.ExtendSectorExpirationParams
	PreCommitSectorBatchParams   = miner8.PreCommitSectorBatchParams
	TerminationDeclaration       = miner8.TerminationDeclaration
	TerminateSectorsParams       = miner8.TerminateSectorsParams
	WithdrawBalanceParams        = miner8.WithdrawBalanceParams

	CreateMinerParams = power8.CreateMinerParams
	CreateMinerReturn = power8.CreateMinerReturn

	AggregateSealVerifyInfo          = proof8.AggregateSealVerifyInfo
	AggregateSealVerifyProofAndInfos = proof8.AggregateSealVerifyProofAndInfos
	SealVerifyInfo                   = proof8.SealVerifyInfo
	WindowPoStVerifyInfo             = proof8.WindowPoStVerifyInfo
)

const MinAggregatedSectors = miner8.MinAggregatedSectors

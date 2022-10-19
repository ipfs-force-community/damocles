package core

import (
	power8 "github.com/filecoin-project/go-state-types/builtin/v8/power" // power8 struct not implement serialize
	market9 "github.com/filecoin-project/go-state-types/builtin/v9/market"
	miner9 "github.com/filecoin-project/go-state-types/builtin/v9/miner"
	proof9 "github.com/filecoin-project/go-state-types/proof"
)

type (
	ComputeDataCommitmentParams = market9.ComputeDataCommitmentParams
	ComputeDataCommitmentReturn = market9.ComputeDataCommitmentReturn
	SectorDataSpec              = market9.SectorDataSpec

	ChangeWorkerAddressParams    = miner9.ChangeWorkerAddressParams
	CompactSectorNumbersParams   = miner9.CompactSectorNumbersParams
	ExpirationExtension          = miner9.ExpirationExtension
	ExtendSectorExpirationParams = miner9.ExtendSectorExpirationParams
	PreCommitSectorBatchParams   = miner9.PreCommitSectorBatchParams
	TerminationDeclaration       = miner9.TerminationDeclaration
	TerminateSectorsParams       = miner9.TerminateSectorsParams
	WithdrawBalanceParams        = miner9.WithdrawBalanceParams

	CreateMinerParams = power8.CreateMinerParams
	CreateMinerReturn = power8.CreateMinerReturn

	AggregateSealVerifyInfo          = proof9.AggregateSealVerifyInfo
	AggregateSealVerifyProofAndInfos = proof9.AggregateSealVerifyProofAndInfos
	SealVerifyInfo                   = proof9.SealVerifyInfo
	WindowPoStVerifyInfo             = proof9.WindowPoStVerifyInfo
)

const MinAggregatedSectors = miner9.MinAggregatedSectors

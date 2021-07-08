package commitmgr

import (
	"bytes"
	"context"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	messager "github.com/filecoin-project/venus-messager/types"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	"golang.org/x/xerrors"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

func pushPreCommitSingle(ctx context.Context, ds api.SectorsDatastore, msgClient venusMessager.IMessager,
	from address.Address, s abi.SectorID, spec messager.MsgMeta, stateMgr SealingAPI) error {
	sector, err := ds.GetSector(ctx, s)
	if err != nil {
		log.Infof("Sector %v not found in db, there must be something weird", s)
		return err
	}
	params, deposit, _, err := preCommitParams(ctx, stateMgr, sector)
	if err != nil {
		return err
	}
	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return xerrors.Errorf("serialize pre-commit sector parameters: %w", err)
	}

	to, err := address.NewIDAddress(uint64(sector.SectorID.Miner))
	if err != nil {
		return xerrors.Errorf("marshal into to failed %w", err)
	}

	mcid, err := pushMessage(ctx, from, to, deposit, miner.Methods.ProveCommitSector, msgClient, spec, enc.Bytes())
	if err != nil {
		return err
	}
	log.Infof("precommit of sector %d sent cid: %s", s.Number, mcid)

	sector.PreCommitCid = &mcid
	sector.NeedSend = false
	return ds.PutSector(ctx, sector)
}

func pushCommitSingle(ctx context.Context, ds api.SectorsDatastore, msgClient venusMessager.IMessager,
	from address.Address, s abi.SectorID, spec messager.MsgMeta, stateMgr SealingAPI) error {
	sector, err := ds.GetSector(ctx, s)
	if err != nil {
		log.Infof("Sector %v not found in db, there must be something weird", s)
		return err
	}

	params := &miner.ProveCommitSectorParams{
		SectorNumber: s.Number,
		Proof:        sector.Proof,
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return xerrors.Errorf("serialize pre-commit sector parameters: %w", err)
	}

	to, err := address.NewIDAddress(uint64(sector.SectorID.Miner))
	if err != nil {
		return xerrors.Errorf("marshal into to failed %w", err)
	}

	tok, _, err := stateMgr.ChainHead(ctx)
	if err != nil {
		return err
	}

	collateral, err := getSectorCollateral(ctx, stateMgr, to, s.Number, tok)
	if err != nil {
		return err
	}

	mcid, err := pushMessage(ctx, from, to, collateral, miner.Methods.ProveCommitSector, msgClient, spec, enc.Bytes())
	if err != nil {
		return err
	}
	log.Infof("prove of sector %d sent cid: %s", s.Number, mcid)
	sector.CommitCid = &mcid
	sector.NeedSend = false
	return ds.PutSector(ctx, sector)
}

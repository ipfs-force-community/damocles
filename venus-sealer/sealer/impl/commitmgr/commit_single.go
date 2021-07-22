package commitmgr

import (
	"bytes"
	"context"
	"fmt"

	"github.com/filecoin-project/go-address"
	venusMessager "github.com/filecoin-project/venus-messager/api/client"
	messager "github.com/filecoin-project/venus-messager/types"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"

	"github.com/dtynn/venus-cluster/venus-sealer/sealer/api"
)

func pushPreCommitSingle(ctx context.Context, msgClient venusMessager.IMessager,
	from, to address.Address, s *api.Sector, spec messager.MsgMeta, stateMgr SealingAPI) error {
	params, deposit, _, err := preCommitParams(ctx, stateMgr, *s)
	if err != nil {
		return err
	}
	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("serialize pre-commit sector parameters: %w", err)
	}

	mcid, err := pushMessage(ctx, from, to, deposit, miner.Methods.ProveCommitSector, msgClient, spec, enc.Bytes())
	if err != nil {
		return err
	}
	log.Infof("precommit of sector %d sent cid: %s", s.SectorID.Number, mcid)

	s.PreCommitCid = &mcid
	return nil
}

func pushCommitSingle(ctx context.Context, msgClient venusMessager.IMessager,
	from, to address.Address, s *api.Sector, spec messager.MsgMeta, stateMgr SealingAPI) error {
	params := &miner.ProveCommitSectorParams{
		SectorNumber: s.SectorID.Number,
		Proof:        s.Proof,
	}

	enc := new(bytes.Buffer)
	if err := params.MarshalCBOR(enc); err != nil {
		return fmt.Errorf("serialize pre-commit sector parameters: %w", err)
	}

	tok, _, err := stateMgr.ChainHead(ctx)
	if err != nil {
		return err
	}

	collateral, err := getSectorCollateral(ctx, stateMgr, to, s.SectorID.Number, tok)
	if err != nil {
		return err
	}

	mcid, err := pushMessage(ctx, from, to, collateral, miner.Methods.ProveCommitSector, msgClient, spec, enc.Bytes())
	if err != nil {
		return err
	}
	log.Infof("prove of sector %d sent cid: %s", s.SectorID.Number, mcid)
	s.CommitCid = &mcid
	return nil
}

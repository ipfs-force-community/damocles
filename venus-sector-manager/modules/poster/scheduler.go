package poster

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/specs-storage/storage"
	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/pkg/specactors/builtin"
	"github.com/filecoin-project/venus/pkg/specactors/builtin/miner"
	specpolicy "github.com/filecoin-project/venus/pkg/specactors/policy"
	"github.com/filecoin-project/venus/pkg/types"
	"github.com/hashicorp/go-multierror"

	"github.com/dtynn/venus-cluster/venus-sector-manager/api"
	"github.com/dtynn/venus-cluster/venus-sector-manager/modules"
	"github.com/dtynn/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/dtynn/venus-cluster/venus-sector-manager/modules/util"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/logging"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/messager"
	"github.com/dtynn/venus-cluster/venus-sector-manager/pkg/objstore"
)

func newScheduler(ctx context.Context, mid abi.ActorID, cfg *modules.SafeConfig, verifier api.Verifier, prover api.Prover, indexer api.SectorIndexer, capi chain.API, rand api.RandomnessAPI, mapi messager.API) (*scheduler, error) {
	maddr, err := address.NewIDAddress(uint64(mid))
	if err != nil {
		return nil, err
	}

	actor := api.ActorIdent{
		Addr: maddr,
		ID:   mid,
	}

	minfo, err := capi.StateMinerInfo(ctx, actor.Addr, types.EmptyTSK)
	if err != nil {
		return nil, fmt.Errorf("get miner info: %w", err)
	}

	return &scheduler{
		actor:     actor,
		proofType: minfo.WindowPoStProofType,
		cfg:       cfg,
		verifier:  verifier,
		prover:    prover,
		indexer:   indexer,
		chain:     capi,
		rand:      rand,
		msg:       mapi,
		clock:     clock.NewSystemClock(),
		log:       log.With("miner", actor.ID),
	}, nil
}

type scheduler struct {
	actor     api.ActorIdent
	proofType abi.RegisteredPoStProof

	cfg      *modules.SafeConfig
	verifier api.Verifier
	prover   api.Prover
	indexer  api.SectorIndexer
	chain    chain.API
	rand     api.RandomnessAPI
	msg      messager.API

	clock clock.Clock
	log   *logging.ZapLogger
}

func (s *scheduler) StateMinerProvingDeadline(
	ctx context.Context,
	addr address.Address,
	tsk types.TipSetKey,
) (*dline.Info, error) {
	return s.chain.StateMinerProvingDeadline(ctx, addr, tsk)
}

func (s *scheduler) failPost(err error, ts *types.TipSet, deadline *dline.Info) {
	s.log.Errorf("Got post err %+v - TODO handle errors", err)
}

func (s *scheduler) onAbort(ts *types.TipSet, deadline *dline.Info) {
	s.log.Warnf("deadline %d has been aborted", deadline.Open)
}

func (s *scheduler) startGeneratePoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	completeGeneratePoST CompleteGeneratePoSTCb,
) context.CancelFunc {
	ctx, abort := context.WithCancel(ctx)
	go func() {
		defer abort()

		posts, err := s.runGeneratePoST(ctx, ts, deadline)
		completeGeneratePoST(posts, err)
	}()

	return abort
}

func (s *scheduler) runGeneratePoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
) ([]miner.SubmitWindowedPoStParams, error) {
	posts, err := s.runPost(ctx, *deadline, ts)
	if err != nil {
		s.log.Errorf("runPost failed: %+v", err)
		return nil, err
	}

	return posts, nil
}

func (s *scheduler) runPost(ctx context.Context, di dline.Info, ts *types.TipSet) ([]miner.SubmitWindowedPoStParams, error) {
	go func() {
		// TODO: extract from runPost, run on fault cutoff boundaries

		// check faults / recoveries for the *next* deadline. It's already too
		// late to declare them for this deadline
		declDeadline := (di.Index + 2) % di.WPoStPeriodDeadlines

		partitions, err := s.chain.StateMinerPartitions(context.TODO(), s.actor.Addr, declDeadline, ts.Key())
		if err != nil {
			s.log.Errorf("getting partitions: %v", err)
			return
		}

		if _, err = s.checkNextRecoveries(context.TODO(), declDeadline, partitions, ts.Key()); err != nil {
			// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
			s.log.Errorf("checking sector recoveries: %v", err)
		}

		if ts.Height() > policy.NetParams.Network.ForkUpgradeParam.UpgradeIgnitionHeight {
			return // FORK: declaring faults after ignition upgrade makes no sense
		}

		if _, err = s.checkNextFaults(context.TODO(), declDeadline, partitions, ts.Key()); err != nil {
			// TODO: This is also potentially really bad, but we try to post anyways
			s.log.Errorf("checking sector faults: %v", err)
		}

	}()

	headTs, err := s.chain.ChainHead(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting current head: %w", err)
	}

	rand, err := s.rand.GetWindowPoStChanlleengeRand(ctx, headTs.Key(), di.Challenge, s.actor.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chain randomness from beacon for window post (ts=%d; deadline=%d): %w", ts.Height(), di, err)
	}

	// Get the partitions for the given deadline
	partitions, err := s.chain.StateMinerPartitions(ctx, s.actor.Addr, di.Index, ts.Key())
	if err != nil {
		return nil, fmt.Errorf("getting partitions: %w", err)
	}

	nv, err := s.chain.StateNetworkVersion(ctx, ts.Key())
	if err != nil {
		return nil, fmt.Errorf("getting network version: %w", err)
	}

	// Split partitions into batches, so as not to exceed the number of sectors
	// allowed in a single message
	partitionBatches, err := s.batchPartitions(partitions, nv)
	if err != nil {
		return nil, err
	}

	// Generate proofs in batches
	posts := make([]miner.SubmitWindowedPoStParams, 0, len(partitionBatches))
	for batchIdx, batch := range partitionBatches {
		batchPartitionStartIdx := 0
		for _, batch := range partitionBatches[:batchIdx] {
			batchPartitionStartIdx += len(batch)
		}

		params := miner.SubmitWindowedPoStParams{
			Deadline:   di.Index,
			Partitions: make([]miner.PoStPartition, 0, len(batch)),
			Proofs:     nil,
		}

		skipCount := uint64(0)
		postSkipped := bitfield.New()
		somethingToProve := false

		// Retry until we run out of sectors to prove.
		for retries := 0; ; retries++ {
			var partitions []miner.PoStPartition
			var sinfos []builtin.SectorInfo
			for partIdx, partition := range batch {
				// TODO: Can do this in parallel
				toProve, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
				if err != nil {
					return nil, fmt.Errorf("removing faults from set of sectors to prove: %w", err)
				}
				toProve, err = bitfield.MergeBitFields(toProve, partition.RecoveringSectors)
				if err != nil {
					return nil, fmt.Errorf("adding recoveries to set of sectors to prove: %w", err)
				}

				good, err := s.checkSectors(ctx, toProve, ts.Key())
				if err != nil {
					return nil, fmt.Errorf("checking sectors to skip: %w", err)
				}

				good, err = bitfield.SubtractBitField(good, postSkipped)
				if err != nil {
					return nil, fmt.Errorf("toProve - postSkipped: %w", err)
				}

				skipped, err := bitfield.SubtractBitField(toProve, good)
				if err != nil {
					return nil, fmt.Errorf("toProve - good: %w", err)
				}

				sc, err := skipped.Count()
				if err != nil {
					return nil, fmt.Errorf("getting skipped sector count: %w", err)
				}

				skipCount += sc

				ssi, err := s.sectorsForProof(ctx, good, partition.AllSectors, ts)
				if err != nil {
					return nil, fmt.Errorf("getting sorted sector info: %w", err)
				}

				if len(ssi) == 0 {
					continue
				}

				sinfos = append(sinfos, ssi...)
				partitions = append(partitions, miner.PoStPartition{
					Index:   uint64(batchPartitionStartIdx + partIdx),
					Skipped: skipped,
				})
			}

			if len(sinfos) == 0 {
				// nothing to prove for this batch
				break
			}

			// Generate proof
			s.log.Infow("running window post",
				"chain-random", rand,
				"deadline", di,
				"height", ts.Height(),
				"skipped", skipCount)

			tsStart := s.clock.Now()

			privSectors, err := s.sectorsPubToPrivate(ctx, sinfos)
			if err != nil {
				return nil, fmt.Errorf("turn public sector infos into private: %w", err)
			}

			postOut, ps, err := s.prover.GenerateWindowPoSt(ctx, s.actor.ID, privSectors, append(abi.PoStRandomness{}, rand.Rand...))
			elapsed := time.Since(tsStart)

			s.log.Infow("computing window post", "batch", batchIdx, "elapsed", elapsed)

			if err == nil {
				// If we proved nothing, something is very wrong.
				if len(postOut) == 0 {
					return nil, fmt.Errorf("received no proofs back from generate window post")
				}

				headTs, err := s.chain.ChainHead(ctx)
				if err != nil {
					return nil, fmt.Errorf("getting current head: %w", err)
				}

				checkRand, err := s.rand.GetWindowPoStChanlleengeRand(ctx, headTs.Key(), di.Challenge, s.actor.ID)
				if err != nil {
					return nil, fmt.Errorf("failed to get chain randomness from beacon for window post (ts=%d; deadline=%d): %w", ts.Height(), di, err)
				}

				if !bytes.Equal(checkRand.Rand, rand.Rand) {
					s.log.Warnw("windowpost randomness changed", "old", rand, "new", checkRand, "ts-height", ts.Height(), "challenge-height", di.Challenge, "tsk", ts.Key())
					continue
				}

				// If we generated an incorrect proof, try again.
				if correct, err := s.verifier.VerifyWindowPoSt(ctx, api.WindowPoStVerifyInfo{
					Randomness:        abi.PoStRandomness(checkRand.Rand),
					Proofs:            postOut,
					ChallengedSectors: sinfos,
					Prover:            s.actor.ID,
				}); err != nil {
					s.log.Errorw("window post verification failed", "post", postOut, "error", err)
					time.Sleep(5 * time.Second)
					continue
				} else if !correct {
					s.log.Errorw("generated incorrect window post proof", "post", postOut, "error", err)
					continue
				}

				// Proof generation successful, stop retrying
				somethingToProve = true
				params.Partitions = partitions
				params.Proofs = postOut
				break
			}

			// Proof generation failed, so retry

			if len(ps) == 0 {
				// If we didn't skip any new sectors, we failed
				// for some other reason and we need to abort.
				return nil, fmt.Errorf("running window post failed: %w", err)
			}
			// TODO: maybe mark these as faulty somewhere?

			s.log.Warnw("generate window post skipped sectors", "sectors", ps, "error", err, "try", retries)

			// Explicitly make sure we haven't aborted this PoSt
			// (GenerateWindowPoSt may or may not check this).
			// Otherwise, we could try to continue proving a
			// deadline after the deadline has ended.
			if ctx.Err() != nil {
				s.log.Warnw("aborting PoSt due to context cancellation", "error", ctx.Err(), "deadline", di.Index)
				return nil, ctx.Err()
			}

			skipCount += uint64(len(ps))
			for _, sector := range ps {
				postSkipped.Set(uint64(sector.Number))
			}
		}

		// Nothing to prove for this batch, try the next batch
		if !somethingToProve {
			continue
		}

		posts = append(posts, params)
	}

	return posts, nil
}

func (s *scheduler) sectorsPubToPrivate(ctx context.Context, sectorInfo []builtin.SectorInfo) (api.SortedPrivateSectorInfo, error) {
	out := make([]api.PrivateSectorInfo, 0, len(sectorInfo))
	for _, sector := range sectorInfo {
		sid := storage.SectorRef{
			ID:        abi.SectorID{Miner: s.actor.ID, Number: sector.SectorNumber},
			ProofType: sector.SealProof,
		}

		postProofType, err := sid.ProofType.RegisteredWindowPoStProof()
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("acquiring registered PoSt proof from sector info %+v: %w", s, err)
		}

		objins, err := s.getObjInstanceForSector(ctx, sid.ID)
		if err != nil {
			return api.SortedPrivateSectorInfo{}, fmt.Errorf("get objstore instance for %s: %w", util.FormatSectorID(sid.ID), err)
		}

		subCache := util.SectorPath(util.SectorPathTypeCache, sid.ID)
		subSealed := util.SectorPath(util.SectorPathTypeSealed, sid.ID)

		out = append(out, api.PrivateSectorInfo{
			CacheDirPath:     objins.FullPath(ctx, subCache),
			PoStProofType:    postProofType,
			SealedSectorPath: objins.FullPath(ctx, subSealed),
			SectorInfo:       sector,
		})
	}

	return api.NewSortedPrivateSectorInfo(out...), nil
}

func (s *scheduler) checkNextFaults(ctx context.Context, dlIdx uint64, partitions []chain.Partition, tsk types.TipSetKey) ([]miner.FaultDeclaration, error) {
	bad := uint64(0)
	params := &miner.DeclareFaultsParams{
		Faults: []miner.FaultDeclaration{},
	}

	for partIdx, partition := range partitions {
		nonFaulty, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
		if err != nil {
			return nil, fmt.Errorf("determining non faulty sectors: %w", err)
		}

		good, err := s.checkSectors(ctx, nonFaulty, tsk)
		if err != nil {
			return nil, fmt.Errorf("checking sectors: %w", err)
		}

		newFaulty, err := bitfield.SubtractBitField(nonFaulty, good)
		if err != nil {
			return nil, fmt.Errorf("calculating faulty sector set: %w", err)
		}

		c, err := newFaulty.Count()
		if err != nil {
			return nil, fmt.Errorf("counting faulty sectors: %w", err)
		}

		if c == 0 {
			continue
		}

		bad += c

		params.Faults = append(params.Faults, miner.FaultDeclaration{
			Deadline:  dlIdx,
			Partition: uint64(partIdx),
			Sectors:   newFaulty,
		})
	}

	faults := params.Faults
	if len(faults) == 0 {
		return faults, nil
	}

	s.log.Errorw("DETECTED FAULTY SECTORS, declaring faults", "count", bad)

	uid, waitCh, err := s.publishMessage(ctx, miner.Methods.DeclareFaults, params, true)
	if err != nil {
		return faults, err
	}

	s.log.Warnw("declare faults message published", "uid", uid)

	res := <-waitCh

	if res.err != nil {
		return faults, fmt.Errorf("declare faults wait error: %w", res.err)
	}

	if res.Message.Receipt.ExitCode != 0 {
		return faults, fmt.Errorf("declare faults msg %s got non-0 exit code: %d", res.Message.SignedCid, res.Message.Receipt.ExitCode)
	}

	return faults, nil
}

func (s *scheduler) checkNextRecoveries(ctx context.Context, dlIdx uint64, partitions []chain.Partition, tsk types.TipSetKey) ([]miner.RecoveryDeclaration, error) {
	faulty := uint64(0)
	params := &miner.DeclareFaultsRecoveredParams{
		Recoveries: []miner.RecoveryDeclaration{},
	}

	for partIdx, partition := range partitions {
		unrecovered, err := bitfield.SubtractBitField(partition.FaultySectors, partition.RecoveringSectors)
		if err != nil {
			return nil, fmt.Errorf("subtracting recovered set from fault set: %w", err)
		}

		uc, err := unrecovered.Count()
		if err != nil {
			return nil, fmt.Errorf("counting unrecovered sectors: %w", err)
		}

		if uc == 0 {
			continue
		}

		faulty += uc

		recovered, err := s.checkSectors(ctx, unrecovered, tsk)
		if err != nil {
			return nil, fmt.Errorf("checking unrecovered sectors: %w", err)
		}

		// if all sectors failed to recover, don't declare recoveries
		recoveredCount, err := recovered.Count()
		if err != nil {
			return nil, fmt.Errorf("counting recovered sectors: %w", err)
		}

		if recoveredCount == 0 {
			continue
		}

		params.Recoveries = append(params.Recoveries, miner.RecoveryDeclaration{
			Deadline:  dlIdx,
			Partition: uint64(partIdx),
			Sectors:   recovered,
		})
	}

	recoveries := params.Recoveries
	if len(recoveries) == 0 {
		if faulty != 0 {
			s.log.Warnw("No recoveries to declare", "deadline", dlIdx, "faulty", faulty)
		}

		return recoveries, nil
	}

	_, resCh, err := s.publishMessage(ctx, miner.Methods.DeclareFaultsRecovered, params, true)
	if err != nil {
		return recoveries, err
	}

	res := <-resCh

	if res.err != nil {
		return recoveries, fmt.Errorf("declare faults recovered wait error: %w", res.err)
	}

	if res.Message.Receipt.ExitCode != 0 {
		return recoveries, fmt.Errorf("declare faults recovered %s got non-0 exit code: %d", res.Message.SignedCid, res.Message.Receipt.ExitCode)
	}

	return recoveries, nil
}

func (s *scheduler) checkSectors(ctx context.Context, check bitfield.BitField, tsk types.TipSetKey) (bitfield.BitField, error) {
	sectorInfos, err := s.chain.StateMinerSectors(ctx, s.actor.Addr, &check, tsk)
	if err != nil {
		return bitfield.BitField{}, err
	}

	sectors := make(map[abi.SectorNumber]struct{})
	var tocheck []storage.SectorRef
	for _, info := range sectorInfos {
		sectors[info.SectorNumber] = struct{}{}
		tocheck = append(tocheck, storage.SectorRef{
			ProofType: info.SealProof,
			ID: abi.SectorID{
				Miner:  s.actor.ID,
				Number: info.SectorNumber,
			},
		})
	}

	bad, err := s.checkProvable(ctx, tocheck)
	if err != nil {
		return bitfield.BitField{}, fmt.Errorf("checking provable sectors: %w", err)
	}

	for id := range bad {
		delete(sectors, id.Number)
	}

	s.log.Warnw("Checked sectors", "checked", len(tocheck), "good", len(sectors))

	sbf := bitfield.New()
	for s := range sectors {
		sbf.Set(uint64(s))
	}

	return sbf, nil
}

func (s *scheduler) checkProvable(ctx context.Context, targets []storage.SectorRef) (map[abi.SectorID]string, error) {
	strict := postPolicyFromConfig(s.actor.ID, s.cfg).StrictCheck

	results := make([]string, len(targets))
	var wg sync.WaitGroup
	wg.Add(len(targets))

	for ti := range targets {
		go func(i int) {
			var reason string
			defer func() {
				if reason != "" {
					results[i] = reason
				}

				wg.Done()
			}()

			sid := targets[i].ID
			objins, err := s.getObjInstanceForSector(ctx, sid)
			if err != nil {
				reason = fmt.Sprintf("get objstore instance for %s: %s", util.FormatSectorID(sid), err)
				return
			}

			subSealed := util.SectorPath(util.SectorPathTypeSealed, sid)
			_, err = objins.Stat(ctx, subSealed)
			if err != nil {
				reason = fmt.Sprintf("get stat info for %s: %s", util.FormatSectorID(sid), err)
				return
			}

			if !strict {
				return
			}

			// TODO more strictly checks

			return

		}(ti)
	}

	wg.Wait()

	bad := map[abi.SectorID]string{}
	for ri := range results {
		if results[ri] != "" {
			bad[targets[ri].ID] = results[ri]
		}
	}

	return bad, nil
}

func (s *scheduler) getObjInstanceForSector(ctx context.Context, sid abi.SectorID) (objstore.Store, error) {
	insname, has, err := s.indexer.Find(ctx, sid)
	if err != nil {
		return nil, fmt.Errorf("find objstore instance: %w", err)
	}

	if !has {
		return nil, fmt.Errorf("objstore instance not found")
	}

	instance, err := s.indexer.StoreMgr().GetInstance(ctx, insname)
	if err != nil {
		return nil, fmt.Errorf("get objstore instance %s: %w", insname, err)
	}

	return instance, nil
}

func (s *scheduler) sectorsForProof(ctx context.Context, goodSectors, allSectors bitfield.BitField, ts *types.TipSet) ([]builtin.SectorInfo, error) {
	sset, err := s.chain.StateMinerSectors(ctx, s.actor.Addr, &goodSectors, ts.Key())
	if err != nil {
		return nil, err
	}

	if len(sset) == 0 {
		return nil, nil
	}

	substitute := builtin.SectorInfo{
		SectorNumber: sset[0].SectorNumber,
		SealedCID:    sset[0].SealedCID,
		SealProof:    sset[0].SealProof,
	}

	sectorByID := make(map[uint64]builtin.SectorInfo, len(sset))
	for _, sector := range sset {
		sectorByID[uint64(sector.SectorNumber)] = builtin.SectorInfo{
			SectorNumber: sector.SectorNumber,
			SealedCID:    sector.SealedCID,
			SealProof:    sector.SealProof,
		}
	}

	proofSectors := make([]builtin.SectorInfo, 0, len(sset))
	if err := allSectors.ForEach(func(sectorNo uint64) error {
		if info, found := sectorByID[sectorNo]; found {
			proofSectors = append(proofSectors, info)
		} else {
			proofSectors = append(proofSectors, substitute)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("iterating partition sector bitmap: %w", err)
	}

	return proofSectors, nil
}

func (s *scheduler) batchPartitions(partitions []chain.Partition, nv network.Version) ([][]chain.Partition, error) {
	// We don't want to exceed the number of sectors allowed in a message.
	// So given the number of sectors in a partition, work out the number of
	// partitions that can be in a message without exceeding sectors per
	// message:
	// floor(number of sectors allowed in a message / sectors per partition)
	// eg:
	// max sectors per message  7:  ooooooo
	// sectors per partition    3:  ooo
	// partitions per message   2:  oooOOO
	//                              <1><2> (3rd doesn't fit)
	partitionsPerMsg, err := specpolicy.GetMaxPoStPartitions(nv, s.proofType)
	if err != nil {
		return nil, fmt.Errorf("getting sectors per partition: %w", err)
	}

	// Also respect the AddressedPartitionsMax (which is the same as DeclarationsMax (which is all really just MaxPartitionsPerDeadline))
	if partitionsPerMsg > specpolicy.GetDeclarationsMax(nv) {
		partitionsPerMsg = specpolicy.GetDeclarationsMax(nv)
	}

	// The number of messages will be:
	// ceiling(number of partitions / partitions per message)
	batchCount := len(partitions) / partitionsPerMsg
	if len(partitions)%partitionsPerMsg != 0 {
		batchCount++
	}

	// Split the partitions into batches
	batches := make([][]chain.Partition, 0, batchCount)
	for i := 0; i < len(partitions); i += partitionsPerMsg {
		end := i + partitionsPerMsg
		if end > len(partitions) {
			end = len(partitions)
		}
		batches = append(batches, partitions[i:end])
	}

	return batches, nil
}

func (s *scheduler) startSubmitPoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	posts []miner.SubmitWindowedPoStParams,
	completeSubmitPoST CompleteSubmitPoSTCb,
) context.CancelFunc {

	ctx, abort := context.WithCancel(ctx)
	go func() {
		defer abort()

		err := s.runSubmitPoST(ctx, ts, deadline, posts)
		completeSubmitPoST(err)
	}()

	return abort
}

func (s *scheduler) runSubmitPoST(
	ctx context.Context,
	ts *types.TipSet,
	deadline *dline.Info,
	posts []miner.SubmitWindowedPoStParams,
) error {
	if len(posts) == 0 {
		return nil
	}

	// Get randomness from tickets
	// use the challenge epoch if we've upgraded to network version 4
	// (actors version 2). We want to go back as far as possible to be safe.
	commEpoch := deadline.Open
	if ver, err := s.chain.StateNetworkVersion(ctx, types.EmptyTSK); err != nil {
		s.log.Errorw("failed to get network version to determine PoSt epoch randomness lookback", "error", err)
	} else if ver >= network.Version4 {
		commEpoch = deadline.Challenge
	}

	commRand, err := s.rand.GetWindowPoStCommitRand(ctx, ts.Key(), commEpoch)
	if err != nil {
		err = fmt.Errorf("failed to get chain randomness from tickets for windowPost (ts=%d; deadline=%d): %w", ts.Height(), commEpoch, err)
		s.log.Errorf("submitPost failed: %+v", err)

		return err
	}

	var submitErrs error
	for i := range posts {
		// Add randomness to PoST
		post := &posts[i]
		post.ChainCommitEpoch = commEpoch
		post.ChainCommitRand = commRand.Rand

		// Submit PoST
		err := s.submitPost(ctx, post)
		if err != nil {
			s.log.Errorf("submit window post failed: %+v", err)
			submitErrs = multierror.Append(submitErrs, err)
		}
	}

	return submitErrs
}

func (s *scheduler) submitPost(ctx context.Context, proof *miner.SubmitWindowedPoStParams) error {
	uid, resCh, err := s.publishMessage(ctx, miner.Methods.SubmitWindowedPoSt, proof, true)
	if err != nil {
		return fmt.Errorf("publish window post message: %w", err)
	}

	s.log.Infof("Submitted window post: %s", uid)

	go func() {
		res := <-resCh
		if res.err != nil {
			s.log.Errorf("wait for message result falied: %s", err)
			return
		}

		if res.Message.Receipt.ExitCode != 0 {
			s.log.Errorf("Submitting window post %s failed: exit %d", res.SignedCid, res.Message.Receipt.ExitCode)
			return
		}

	}()

	return nil
}

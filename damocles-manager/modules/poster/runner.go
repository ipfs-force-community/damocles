package poster

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v12/miner"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/filecoin-project/venus/venus-shared/types/messager"
	"github.com/ipfs-force-community/damocles/damocles-manager/core"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/policy"
	"github.com/ipfs-force-community/damocles/damocles-manager/modules/util"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/chain"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/slices"
)

const (
	EventTick EventType = "tick"
)

var slog = logging.New("subscriber")

type postExecutor interface {
	HandleFaults(ts *types.TipSet)
	Prepare() (*PrepareResult, error)
	GeneratePoSt(*PrepareResult) ([]miner.SubmitWindowedPoStParams, error)
	SubmitPoSts(ts *types.TipSet, proofs []miner.SubmitWindowedPoStParams) error
}

var NewPostExecutor = func(ctx context.Context, deps postDeps, mid abi.ActorID, maddr address.Address, proofType abi.RegisteredPoStProof, dinfo *dline.Info, cfg *modules.MinerPoStConfig) postExecutor {
	logger := log.With("deadline", dinfo.Index, "open", dinfo.Open, "close", dinfo.Close, "challenge", dinfo.Challenge)
	executor := &postRunner{
		deps:      deps,
		mid:       mid,
		maddr:     maddr,
		proofType: proofType,

		dinfo: dinfo,
		log:   logger,
		ctx:   ctx,
		cfg:   cfg,
	}
	return executor
}

type postRunner struct {
	mock bool
	deps postDeps

	mid       abi.ActorID
	maddr     address.Address
	proofType abi.RegisteredPoStProof

	ctx   context.Context
	dinfo *dline.Info
	log   *logging.ZapLogger
	cfg   *modules.MinerPoStConfig

	// prepareResult *prepareResult
}

type PrepareResult struct {
	rand           core.WindowPoStRandomness
	partitionBatch [][]types.Partition
}

func (pr *postRunner) SubmitPoSts(ts *types.TipSet, proofs []miner.SubmitWindowedPoStParams) error {
	tsk := ts.Key()
	tsh := ts.Height()
	slog := pr.log.With("stage", "submit-post", "posts", len(proofs), "tsk", tsk, "tsh", tsh)

	// Get randomness from tickets
	// use the challenge epoch if we've upgraded to network version 4
	// (actors version 2). We want to go back as far as possible to be safe.
	commEpoch := pr.dinfo.Open
	if ver, err := pr.deps.chain.StateNetworkVersion(pr.ctx, ts.Key()); err != nil {
		slog.Errorf("get network version to determine PoSt epoch randomness lookback: %v", err)
	} else if ver >= network.Version4 {
		commEpoch = pr.dinfo.Challenge
	}

	slog = slog.With("comm-epoch", commEpoch)

	commRand, err := pr.deps.rand.GetWindowPoStCommitRand(pr.ctx, ts.Key(), commEpoch)
	if err != nil {
		return fmt.Errorf("get chain randomness from beacon for windowPost: %w", err)
	}

	errs := make(chan error, len(proofs))
	wg := sync.WaitGroup{}

	for pi := range proofs {
		post := proofs[pi]
		if len(post.Partitions) == 0 || len(post.Proofs) == 0 {
			continue
		}

		post.ChainCommitEpoch = commEpoch
		post.ChainCommitRand = commRand.Rand

		wg.Add(1)
		go func() {
			defer wg.Done()
			err := pr.submitSinglePost(slog, &post)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	if len(errs) > 0 {
		errInfo := "batch error: \n"
		for err := range errs {
			errInfo += fmt.Sprintf("	* %s\n", err)
		}
		return fmt.Errorf("%s", errInfo)
	}

	return nil
}

func (pr *postRunner) submitSinglePost(slog *logging.ZapLogger, proof *miner.SubmitWindowedPoStParams) error {
	// to avoid being cancelled by proving period detection, use context.Background here
	uid, resCh, err := pr.publishMessage(stbuiltin.MethodsMiner.SubmitWindowedPoSt, proof, false)
	if err != nil {
		return fmt.Errorf("publish post message: %v", err)
	}

	wlog := slog.With("msg-id", uid)
	wlog.Infof("Submitted window post: %s", uid)

	waitCtx, waitCancel := context.WithTimeout(pr.ctx, 30*time.Minute)
	defer waitCancel()

	select {
	case <-waitCtx.Done():
		return fmt.Errorf("wait message result: %w", waitCtx.Err())

	case res := <-resCh:
		if res.err != nil {
			return fmt.Errorf("wait for message result failed: %s", res.err)
		}
		wlog.Infof("window post message succeeded: %s", res.msg.SignedCid)
	}

	return nil
}

func (pr *postRunner) GeneratePoSt(res *PrepareResult) ([]miner.SubmitWindowedPoStParams, error) {
	partitionBatches := res.partitionBatch
	rand := res.rand

	wg := sync.WaitGroup{}
	errs := make(chan error, len(partitionBatches))
	proofs := make(chan miner.SubmitWindowedPoStParams, len(partitionBatches))
	doGenerate := func(batch []chain.Partition, batchPartitionStartIdx int) {
		var err error
		var proof miner.SubmitWindowedPoStParams
		if pr.cfg.Parallel {
			wg.Add(1)
			go func() {
				defer wg.Done()
				proof, err = pr.generatePoStForPartitionBatch(rand, batch, batchPartitionStartIdx)
			}()
		} else {
			proof, err = pr.generatePoStForPartitionBatch(rand, batch, batchPartitionStartIdx)
		}
		if err != nil {
			err = fmt.Errorf("gen wdpost proof for partitions(%d-%d): %w", batchPartitionStartIdx, batchPartitionStartIdx+len(batch), err)
			return
		}

		proofs <- proof
	}

	batchPartitionStartIdx := 0
	for batchIdx := range partitionBatches {
		batch := partitionBatches[batchIdx]
		doGenerate(batch, batchPartitionStartIdx)
		batchPartitionStartIdx += len(batch)
	}
	wg.Wait()
	close(proofs)
	close(errs)

	if len(errs) > 0 {
		errInfo := "batch error: \n"
		for err := range errs {
			errInfo += fmt.Sprintf("	* %s\n", err)
		}
		return nil, fmt.Errorf("%s", errInfo)
	}

	ret := make([]miner.SubmitWindowedPoStParams, 0, len(proofs))
	for proof := range proofs {
		ret = append(ret, proof)
	}

	return ret, nil
}

func (pr *postRunner) generatePoStForPartitionBatch(rand core.WindowPoStRandomness, batch []chain.Partition, batchPartitionStartIdx int) (miner.SubmitWindowedPoStParams, error) {
	log := pr.log.With("partition-start", batchPartitionStartIdx, "batch-count", len(batch))

	params := miner.SubmitWindowedPoStParams{
		Deadline:   pr.dinfo.Index,
		Partitions: make([]miner.PoStPartition, 0, len(batch)),
		Proofs:     nil,
	}

	skipCount := uint64(0)
	postSkipped := bitfield.New()

	proveAttempt := func() error {
		var partitions []miner.PoStPartition
		var xsinfos []builtin.ExtendedSectorInfo
		for partIdx, partition := range batch {
			// TODO: Can do this in parallel
			toProve, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
			if err != nil {
				return fmt.Errorf("removing faults from set of sectors to prove: %w", err)
			}
			toProve, err = bitfield.MergeBitFields(toProve, partition.RecoveringSectors)
			if err != nil {
				return fmt.Errorf("adding recoveries to set of sectors to prove: %w", err)
			}

			good, err := pr.checkSectors(log, toProve)
			if err != nil {
				return fmt.Errorf("checking sectors to skip: %w", err)
			}

			good, err = bitfield.SubtractBitField(good, postSkipped)
			if err != nil {
				return fmt.Errorf("toProve - postSkipped: %w", err)
			}

			skipped, err := bitfield.SubtractBitField(toProve, good)
			if err != nil {
				return fmt.Errorf("toProve - good: %w", err)
			}

			sc, err := skipped.Count()
			if err != nil {
				return fmt.Errorf("getting skipped sector count: %w", err)
			}

			skipCount += sc

			ssi, err := pr.sectorsForProof(good, partition.AllSectors)
			if err != nil {
				return fmt.Errorf("getting sorted sector info: %w", err)
			}

			if len(ssi) == 0 {
				continue
			}

			xsinfos = append(xsinfos, ssi...)
			partitions = append(partitions, miner.PoStPartition{
				Index:   uint64(batchPartitionStartIdx + partIdx),
				Skipped: skipped,
			})
		}

		if len(xsinfos) == 0 {
			// nothing to prove for this batch
			log.Warn("no good sector to prove")
			return nil
		}

		// Generate proof
		log.Infow("running window post",
			"chain-random", rand,
			"skipped", skipCount)

		tsStart := pr.deps.clock.Now()

		// TODO: Drop after nv19 comes and goes
		nv, err := pr.deps.chain.StateNetworkVersion(pr.ctx, types.EmptyTSK)
		if err != nil {
			return fmt.Errorf("get network version: %w", err)
		}

		pp, err := xsinfos[0].SealProof.RegisteredWindowPoStProofByNetworkVersion(nv)
		if err != nil {
			return fmt.Errorf("convert to v1_1 post proof: %w", err)
		}

		proverParams := core.GenerateWindowPoStParams{
			DeadlineIdx: pr.dinfo.Index,
			MinerID:     pr.mid,
			ProofType:   pp,
			Partitions: slices.Map(partitions, func(p miner.PoStPartition) uint64 {
				return p.Index
			}),
			Sectors:    xsinfos,
			Randomness: append(abi.PoStRandomness{}, rand.Rand...),
		}
		postOut, ps, err := pr.deps.prover.GenerateWindowPoSt(pr.ctx, proverParams)

		log.Infow("computing window post", "elapsed", time.Since(tsStart))

		if err == nil {
			// If we proved nothing, something is very wrong.
			if len(postOut) == 0 {
				return fmt.Errorf("received no proofs back from generate window post")
			}

			headTs, err := pr.deps.chain.ChainHead(pr.ctx)
			if err != nil {
				return fmt.Errorf("getting current head: %w", err)
			}

			checkRand, err := pr.deps.rand.GetWindowPoStChanlleengeRand(pr.ctx, headTs.Key(), pr.dinfo.Challenge, pr.mid)
			if err != nil {
				return fmt.Errorf("get chain randomness for checking from beacon for window post: %w", err)
			}

			if !bytes.Equal(checkRand.Rand, rand.Rand) {
				log.Warnw("windowpost randomness changed", "old", rand, "new", checkRand)
				return nil
			}

			// If we generated an incorrect proof, try again.
			sinfos := make([]builtin.SectorInfo, len(xsinfos))
			for i, xsi := range xsinfos {
				sinfos[i] = builtin.SectorInfo{
					SealProof:    xsi.SealProof,
					SectorNumber: xsi.SectorNumber,
					SealedCID:    xsi.SealedCID,
				}
			}

			if correct, err := pr.deps.verifier.VerifyWindowPoSt(pr.ctx, core.WindowPoStVerifyInfo{
				Randomness:        abi.PoStRandomness(checkRand.Rand),
				Proofs:            postOut,
				ChallengedSectors: sinfos,
				Prover:            pr.mid,
			}); err != nil {
				return fmt.Errorf("window post verification failed for %v: %w", postOut, err)
			} else if !correct {
				return fmt.Errorf("incorrect window post proof for %v", postOut)
			}

			// Proof generation successful, stop retrying
			params.Partitions = partitions
			params.Proofs = postOut
			return nil
		}

		// Proof generation failed, so retry
		if len(ps) == 0 {
			// If we didn't skip any new sectors, we failed
			// for some other reason and we need to abort.
			return fmt.Errorf("running window post failed: %w", err)
		}
		// TODO: maybe mark these as faulty somewhere?

		log.Warnw("skipped sectors", "sectors", ps)

		// Explicitly make sure we haven't aborted this PoSt
		// (GenerateWindowPoSt may or may not check this).
		// Otherwise, we could try to continue proving a
		// deadline after the deadline has ended.
		if cerr := pr.ctx.Err(); cerr != nil {
			return cerr
		}

		skipCount += uint64(len(ps))
		for _, sector := range ps {
			postSkipped.Set(uint64(sector.Number))
		}

		return err
	}

	err := proveAttempt()
	return params, err
}

func (pr *postRunner) sectorsForProof(goodSectors, allSectors bitfield.BitField) ([]builtin.ExtendedSectorInfo, error) {
	sset, err := pr.deps.chain.StateMinerSectors(pr.ctx, pr.maddr, &goodSectors, types.EmptyTSK)
	if err != nil {
		return nil, err
	}

	if len(sset) == 0 {
		return nil, nil
	}

	substitute := util.SectorOnChainInfoToExtended(sset[0])

	sectorByID := make(map[uint64]builtin.ExtendedSectorInfo, len(sset))
	for _, sector := range sset {
		sectorByID[uint64(sector.SectorNumber)] = util.SectorOnChainInfoToExtended(sector)
	}

	proofSectors := make([]builtin.ExtendedSectorInfo, 0, len(sset))
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

// TODO: tests
func (pr *postRunner) batchPartitions(partitions []chain.Partition, nv network.Version) ([][]chain.Partition, error) {
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
	partitionsPerMsg, err := specpolicy.GetMaxPoStPartitions(nv, pr.proofType)
	if err != nil {
		return nil, fmt.Errorf("getting sectors per partition: %w", err)
	}

	declMax, err := specpolicy.GetDeclarationsMax(nv)
	if err != nil {
		return nil, fmt.Errorf("getting max declarations: %w", err)
	}

	if partitionsPerMsg > declMax {
		partitionsPerMsg = declMax
	}

	if max := int(pr.cfg.MaxPartitionsPerPoStMessage); max > 0 && partitionsPerMsg > max {
		partitionsPerMsg = max
	}

	batchCount := (len(partitions) + partitionsPerMsg - 1) / partitionsPerMsg

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

func (pr *postRunner) HandleFaults(ts *types.TipSet) {
	declDeadlineIndex := (pr.dinfo.Index + 2) % pr.dinfo.WPoStPeriodDeadlines
	hflog := pr.log.With("decl-index", declDeadlineIndex)

	partitions, err := pr.deps.chain.StateMinerPartitions(pr.ctx, pr.maddr, declDeadlineIndex, ts.Key())
	if err != nil {
		hflog.Errorf("get partitions: %v", err)
		return
	}

	if err := pr.checkRecoveries(hflog, declDeadlineIndex, partitions); err != nil {
		// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
		hflog.Errorf("checking sector recoveries: %v", err)
	}

	if ts.Height() > policy.NetParams.ForkUpgradeParams.UpgradeIgnitionHeight {
		return // FORK: declaring faults after ignition upgrade makes no sense
	}

	if err := pr.checkFaults(hflog, declDeadlineIndex, partitions); err != nil {
		// TODO: This is also potentially really bad, but we try to post anyways
		hflog.Errorf("checking sector faults: %v", err)
	}

	return
}

func (pr *postRunner) checkRecoveries(l *logging.ZapLogger, declIndex uint64, partitions []chain.Partition) error {
	cklog := l.With("stage", "check-recoveries")

	newParams := func() *miner.DeclareFaultsRecoveredParams {
		return &miner.DeclareFaultsRecoveredParams{
			Recoveries: []miner.RecoveryDeclaration{},
		}
	}

	handleRecoverMessage := func(params *miner.DeclareFaultsRecoveredParams) {
		if len(params.Recoveries) == 0 {
			return
		}

		partitionIndexs := make([]string, len(params.Recoveries))
		for ri := range params.Recoveries {
			partitionIndexs[ri] = strconv.FormatUint(params.Recoveries[ri].Partition, 10)
		}

		hlog := cklog.With("partitions", strings.Join(partitionIndexs, ", "))

		messageID, resCh, err := pr.publishMessage(stbuiltin.MethodsMiner.DeclareFaultsRecovered, params, true)
		if err != nil {
			hlog.Errorf("publish message: %s", err)
			return
		}

		hlog = hlog.With("message-id", messageID)

		hlog.Warn("declare faults recovered message published")

		res := <-resCh

		if res.err != nil {
			hlog.Errorf("declare faults recovered wait error: %s", res.err)
			return
		}
	}

	faulty := uint64(0)
	currentParams := newParams()

	var recoverTotal uint64

	for partIdx, partition := range partitions {
		plog := cklog.With("partition", partIdx)

		unrecovered, err := bitfield.SubtractBitField(partition.FaultySectors, partition.RecoveringSectors)
		if err != nil {
			plog.Warnf("subtracting recovered set from fault set: %v", err)
			continue
		}

		uc, err := unrecovered.Count()
		if err != nil {
			plog.Warnf("counting unrecovered sectors: %v", err)
			continue
		}

		if uc == 0 {
			continue
		}

		faulty += uc

		recovered, err := pr.checkSectors(plog, unrecovered)
		if err != nil {
			plog.Errorf("checking unrecovered sectors: %v", err)
			continue
		}

		// if all sectors failed to recover, don't declare recoveries
		recoveredCount, err := recovered.Count()
		if err != nil {
			plog.Warnf("counting recovered sectors: %v", err)
			continue
		}

		if recoveredCount == 0 {
			continue
		}

		// rules to follow if we have indicated that we don't want to recover more than X sectors in a deadline
		cfg := pr.cfg
		recoverSectorsLimit := cfg.MaxRecoverSectorLimit
		if recoverSectorsLimit > 0 {
			// something weird happened, break because we can't recover any more
			if recoverSectorsLimit < recoverTotal {
				log.Warnf("accepted more recoveries (%d) than RecoveringSectorLimit (%d)", recoverTotal, recoverSectorsLimit)
				break
			}

			maxNewRecoverable := recoverSectorsLimit - recoverTotal

			// we need to trim the recover bitfield
			if recoveredCount > maxNewRecoverable {
				recoverySlice, err := recovered.All(math.MaxUint64)
				if err != nil {
					log.Errorw("failed to slice recovery bitfield, breaking out of recovery loop", err)
					break
				}

				log.Warnf("only adding %d sectors to respect RecoveringSectorLimit %d", maxNewRecoverable, recoverSectorsLimit)

				recovered = bitfield.NewFromSet(recoverySlice[:maxNewRecoverable])
				recoveredCount = maxNewRecoverable
			}
		}

		recoverTotal += recoveredCount

		currentParams.Recoveries = append(currentParams.Recoveries, miner.RecoveryDeclaration{
			Deadline:  declIndex,
			Partition: uint64(partIdx),
			Sectors:   recovered,
		})

		if max := cfg.MaxPartitionsPerRecoveryMessage; max > 0 &&
			len(currentParams.Recoveries) >= int(max) {

			go handleRecoverMessage(currentParams)
			currentParams = newParams()
		}

		if recoverSectorsLimit > 0 && recoverTotal >= recoverSectorsLimit {
			log.Warnf("reached recovering sector limit %d, only marking %d sectors for recovery now",
				recoverSectorsLimit,
				recoverTotal)
			break
		}
	}

	if recoverTotal == 0 && faulty != 0 {
		cklog.Warnw("No recoveries to declare", "faulty", faulty)
	}

	go handleRecoverMessage(currentParams)

	return nil
}

func (pr *postRunner) checkFaults(l *logging.ZapLogger, declIndex uint64, partitions []chain.Partition) error {
	cklog := l.With("stage", "check-faults")

	bad := uint64(0)
	params := &miner.DeclareFaultsParams{
		Faults: []miner.FaultDeclaration{},
	}

	for partIdx, partition := range partitions {
		plog := cklog.With("partition", partIdx)

		nonFaulty, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
		if err != nil {
			plog.Warnf("determining non faulty sectors: %v", err)
			continue
		}

		good, err := pr.checkSectors(plog, nonFaulty)
		if err != nil {
			plog.Errorf("checking sectors: %v", err)
			continue
		}

		newFaulty, err := bitfield.SubtractBitField(nonFaulty, good)
		if err != nil {
			plog.Warnf("calculating faulty sector set: %v", err)
			continue
		}

		c, err := newFaulty.Count()
		if err != nil {
			plog.Warnf("counting faulty sectors: %v", err)
			continue
		}

		if c == 0 {
			continue
		}

		bad += c

		params.Faults = append(params.Faults, miner.FaultDeclaration{
			Deadline:  declIndex,
			Partition: uint64(partIdx),
			Sectors:   newFaulty,
		})
	}

	if len(params.Faults) == 0 {
		return nil
	}

	cklog.Errorw("DETECTED FAULTY SECTORS, declaring faults", "count", bad)

	uid, waitCh, err := pr.publishMessage(stbuiltin.MethodsMiner.DeclareFaults, params, true)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	cklog.Warnw("declare faults message published", "message-id", uid)

	res := <-waitCh
	if res.err != nil {
		return fmt.Errorf("declare faults wait error: %w", err)
	}

	return nil
}

func (pr *postRunner) checkSectors(clog *logging.ZapLogger, check bitfield.BitField) (bitfield.BitField, error) {
	sectorInfos, err := pr.deps.chain.StateMinerSectors(pr.ctx, pr.maddr, &check, types.EmptyTSK)
	if err != nil {
		return bitfield.BitField{}, fmt.Errorf("call StateMinerSectors: %w", err)
	}

	sectors := make(map[abi.SectorNumber]struct{})
	var tocheck []builtin.ExtendedSectorInfo
	for _, info := range sectorInfos {
		sectors[info.SectorNumber] = struct{}{}
		tocheck = append(tocheck, util.SectorOnChainInfoToExtended(info))
	}

	// TODO: Drop after nv19 comes and goes
	nv, err := pr.deps.chain.StateNetworkVersion(pr.ctx, types.EmptyTSK)
	if err != nil {
		return bitfield.BitField{}, fmt.Errorf("failed to get network version: %w", err)
	}

	pp := pr.proofType
	if nv >= network.Version19 {
		pp, err = pp.ToV1_1PostProof()
		if err != nil {
			return bitfield.BitField{}, fmt.Errorf("failed to convert to v1_1 post proof: %w", err)
		}
	}

	bad, err := pr.deps.sectorProving.Provable(pr.ctx, pr.mid, pp, tocheck, pr.cfg.StrictCheck, false)
	if err != nil {
		return bitfield.BitField{}, fmt.Errorf("checking provable sectors: %w", err)
	}

	for num := range bad {
		clog.Warnf("bad sector %d: %s", num, bad[num])
		delete(sectors, num)
	}

	clog.Warnw("Checked sectors", "checked", len(tocheck), "good", len(sectors))

	sbf := bitfield.New()
	for s := range sectors {
		sbf.Set(uint64(s))
	}

	return sbf, nil
}

type msgResult struct {
	msg *messager.Message
	err error
}

func (pr *postRunner) publishMessage(method abi.MethodNum, params cbor.Marshaler, useExtraMsgID bool) (string, <-chan msgResult, error) {
	encoded, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return "", nil, fmt.Errorf("serialize params: %w", aerr)
	}

	sender, err := pr.deps.senderSelector.Select(pr.ctx, pr.mid, pr.cfg.GetSenders())
	if err != nil {
		return "", nil, fmt.Errorf("select sender for %d: %w", pr.mid, err)
	}

	msg := types.Message{
		From:      sender,
		To:        pr.maddr,
		Method:    method,
		Params:    encoded,
		Value:     types.NewInt(0),
		GasFeeCap: pr.cfg.GetGasFeeCap().Std(),
	}

	spec := pr.cfg.FeeConfig.GetSendSpec()

	mid := ""
	if !useExtraMsgID {
		mid = msg.Cid().String()
	} else {
		mid = fmt.Sprintf("%s-%v-%v", msg.Cid().String(), pr.dinfo.Index, pr.dinfo.Open)
	}

	if pr.mock {
		ch := make(chan msgResult, 1)
		close(ch)
		return mid, ch, nil
	}

	uid, err := pr.deps.msg.PushMessageWithId(pr.ctx, mid, &msg, &spec)
	if err != nil {
		return "", nil, fmt.Errorf("push msg with id %s: %w", mid, err)
	}

	ch := make(chan msgResult, 1)
	go func() {
		defer close(ch)

		m, err := pr.waitMessage(uid, pr.cfg.Confidence)
		if err != nil {
			ch <- msgResult{
				msg: m,
				err: err,
			}
			return
		}

		if m == nil || m.Receipt == nil {
			ch <- msgResult{
				msg: m,
				err: fmt.Errorf("invalid message returned"),
			}
			return
		}

		if m.Receipt.ExitCode != 0 {
			signed := "nil"
			if m.SignedCid != nil {
				signed = m.SignedCid.String()
			}

			ch <- msgResult{
				msg: m,
				err: fmt.Errorf("got non-zero exit code for %s: %w", signed, m.Receipt.ExitCode),
			}
			return
		}

		ch <- msgResult{
			msg: m,
			err: nil,
		}
	}()

	return uid, ch, nil
}

func (pr *postRunner) waitMessage(mid string, confidence uint64) (*messager.Message, error) {
	return pr.deps.msg.WaitMessage(pr.ctx, mid, confidence)
}

func (pr *postRunner) Prepare() (*PrepareResult, error) {
	tsk := types.EmptyTSK
	rand, err := pr.deps.rand.GetWindowPoStChanlleengeRand(pr.ctx, tsk, pr.dinfo.Challenge, pr.mid)
	if err != nil {
		return nil, fmt.Errorf("getting challenge rand: %w", err)
	}

	partitions, err := pr.deps.chain.StateMinerPartitions(pr.ctx, pr.maddr, pr.dinfo.Index, tsk)
	if err != nil {
		return nil, fmt.Errorf("getting partitions: %w", err)
	}

	nv, err := pr.deps.chain.StateNetworkVersion(pr.ctx, tsk)
	if err != nil {
		return nil, fmt.Errorf("getting network version: %w", err)
	}

	// Split partitions into batches, so as not to exceed the number of sectors
	// allowed in a single message
	partitionBatches, err := pr.batchPartitions(partitions, nv)
	if err != nil {
		return nil, fmt.Errorf("split partitions into batches: %w", err)
	}

	return &PrepareResult{
		rand:           rand,
		partitionBatch: partitionBatches,
	}, nil
}

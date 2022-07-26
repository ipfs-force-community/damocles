package poster

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	stbuiltin "github.com/filecoin-project/go-state-types/builtin"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/cbor"
	"github.com/filecoin-project/go-state-types/dline"
	"github.com/filecoin-project/go-state-types/network"
	"github.com/filecoin-project/venus/pkg/clock"
	"github.com/filecoin-project/venus/venus-shared/actors"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	specpolicy "github.com/filecoin-project/venus/venus-shared/actors/policy"
	"github.com/filecoin-project/venus/venus-shared/types"
	"github.com/filecoin-project/venus/venus-shared/types/messager"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/policy"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/util"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/chain"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var _ postRunnerInner = (*innerRunner)(nil)

type startContext struct {
	ts   *types.TipSet
	pcfg *modules.MinerPoStConfig
}

type proofResult struct {
	sync.Mutex
	proofs []miner.SubmitWindowedPoStParams
	done   int
}

type innerRunner struct {
	pctx postContext

	mid       abi.ActorID
	maddr     address.Address
	proofType abi.RegisteredPoStProof
	dinfo     *dline.Info

	ctx    context.Context
	cancel context.CancelFunc

	sectorTracker core.SectorTracker

	startOnce  sync.Once
	submitOnce sync.Once
	cancelOnce sync.Once

	proofs proofResult

	// log with mid / deadline.Index / deadline.Open / deadline.Close / deadline.Challenge
	log      *logging.ZapLogger
	startCtx startContext
	clock    clock.Clock
	prover   core.Prover
	verifier core.Verifier
}

func (ir *innerRunner) start(pcfg *modules.MinerPoStConfig, ts *types.TipSet) {
	ir.startOnce.Do(func() {
		ir.startCtx.pcfg = pcfg
		ir.startCtx.ts = ts

		baseLog := ir.log.With("tsk", ts.Key(), "tsh", ts.Height())

		go ir.handleFaults(baseLog)
		go ir.generatePoSt(baseLog)
	})
}

func (ir *innerRunner) submit(pcfg *modules.MinerPoStConfig, ts *types.TipSet) {
	// check for proofs
	ir.proofs.Lock()
	proofs := ir.proofs.proofs
	done := ir.proofs.done
	ir.proofs.Unlock()

	if proofs == nil {
		return
	}

	// TODO: submit anyway if current deadline is about to close
	// and if we do this, we should avoid data race for the proofs
	if want := len(proofs); want > done {
		ir.log.Debugw("not all proofs generated", "want", want, "done", done)
		return
	}

	ir.submitOnce.Do(func() {
		ir.submitPoStProofs(pcfg, ts, proofs)
	})
}

func (ir *innerRunner) abort() {
	ir.cancelOnce.Do(func() {
		ir.cancel()
	})
}

func (ir *innerRunner) submitPoStProofs(pcfg *modules.MinerPoStConfig, ts *types.TipSet, proofs []miner.SubmitWindowedPoStParams) {

}

func (ir *innerRunner) generatePoSt(baseLog *logging.ZapLogger) {
	tsk := ir.startCtx.ts.Key()
	glog := baseLog.With("stage", "gen-post")

	rand, err := ir.pctx.rand.GetWindowPoStChanlleengeRand(ir.ctx, tsk, ir.dinfo.Challenge, ir.mid)
	if err != nil {
		glog.Errorf("getting challenge rand: %v", err)
		return
	}

	partitions, err := ir.pctx.chain.StateMinerPartitions(ir.ctx, ir.maddr, ir.dinfo.Index, tsk)
	if err != nil {
		glog.Errorf("getting partitions: %v", err)
		return
	}

	nv, err := ir.pctx.chain.StateNetworkVersion(ir.ctx, tsk)
	if err != nil {
		glog.Errorf("getting network version: %v", err)
		return
	}

	// Split partitions into batches, so as not to exceed the number of sectors
	// allowed in a single message
	partitionBatches, err := ir.batchPartitions(partitions, nv)
	if err != nil {
		glog.Errorf("split partitions into batches: %v", err)
		return
	}

	batchPartitionStartIdx := 0
	for batchIdx := range partitionBatches {
		batch := partitionBatches[batchIdx]

		if ir.startCtx.pcfg.Parallel {
			go ir.generatePoStForPartitionBatch(glog, rand, batchIdx, batch, batchPartitionStartIdx)
		} else {
			ir.generatePoStForPartitionBatch(glog, rand, batchIdx, batch, batchPartitionStartIdx)
		}

		batchPartitionStartIdx += len(batch)
	}

	return
}

func (ir *innerRunner) generatePoStForPartitionBatch(glog *logging.ZapLogger, rand core.WindowPoStRandomness, batchIdx int, batch []chain.Partition, batchPartitionStartIdx int) {

	params := miner.SubmitWindowedPoStParams{
		Deadline:   ir.dinfo.Index,
		Partitions: make([]miner.PoStPartition, 0, len(batch)),
		Proofs:     nil,
	}

	skipCount := uint64(0)
	postSkipped := bitfield.New()

	proveAttemp := func(alog *logging.ZapLogger) (bool, error) {
		var partitions []miner.PoStPartition
		var xsinfos []builtin.ExtendedSectorInfo
		for partIdx, partition := range batch {
			// TODO: Can do this in parallel
			toProve, err := bitfield.SubtractBitField(partition.LiveSectors, partition.FaultySectors)
			if err != nil {
				return false, fmt.Errorf("removing faults from set of sectors to prove: %w", err)
			}
			toProve, err = bitfield.MergeBitFields(toProve, partition.RecoveringSectors)
			if err != nil {
				return false, fmt.Errorf("adding recoveries to set of sectors to prove: %w", err)
			}

			good, err := ir.checkSectors(alog, toProve)
			if err != nil {
				return true, fmt.Errorf("checking sectors to skip: %w", err)
			}

			good, err = bitfield.SubtractBitField(good, postSkipped)
			if err != nil {
				return false, fmt.Errorf("toProve - postSkipped: %w", err)
			}

			skipped, err := bitfield.SubtractBitField(toProve, good)
			if err != nil {
				return false, fmt.Errorf("toProve - good: %w", err)
			}

			sc, err := skipped.Count()
			if err != nil {
				return false, fmt.Errorf("getting skipped sector count: %w", err)
			}

			skipCount += sc

			ssi, err := ir.sectorsForProof(good, partition.AllSectors)
			if err != nil {
				return true, fmt.Errorf("getting sorted sector info: %w", err)
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
			return false, nil
		}

		// Generate proof
		alog.Infow("running window post",
			"chain-random", rand,
			"skipped", skipCount)

		tsStart := ir.clock.Now()

		privSectors, err := ir.sectorTracker.PubToPrivate(ir.ctx, ir.mid, xsinfos, core.SectorWindowPoSt)
		if err != nil {
			return true, fmt.Errorf("turn public sector infos into private: %w", err)
		}

		postOut, ps, err := ir.prover.GenerateWindowPoSt(ir.ctx, ir.mid, core.NewSortedPrivateSectorInfo(privSectors...), append(abi.PoStRandomness{}, rand.Rand...))

		alog.Infow("computing window post", "elapsed", time.Since(tsStart))

		if err == nil {
			// If we proved nothing, something is very wrong.
			if len(postOut) == 0 {
				return false, fmt.Errorf("received no proofs back from generate window post")
			}

			headTs, err := ir.pctx.chain.ChainHead(ir.ctx)
			if err != nil {
				return true, fmt.Errorf("getting current head: %w", err)
			}

			checkRand, err := ir.pctx.rand.GetWindowPoStChanlleengeRand(ir.ctx, headTs.Key(), ir.dinfo.Challenge, ir.mid)
			if err != nil {
				return true, fmt.Errorf("get chain randomness for checking from beacon for window post: %w", err)
			}

			if !bytes.Equal(checkRand.Rand, rand.Rand) {
				alog.Warnw("windowpost randomness changed", "old", rand, "new", checkRand)
				return true, nil
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

			if correct, err := ir.verifier.VerifyWindowPoSt(ir.ctx, core.WindowPoStVerifyInfo{
				Randomness:        abi.PoStRandomness(checkRand.Rand),
				Proofs:            postOut,
				ChallengedSectors: sinfos,
				Prover:            ir.mid,
			}); err != nil {
				return true, fmt.Errorf("window post verification failed for %v: %w", postOut, err)
			} else if !correct {
				return true, fmt.Errorf("incorrect window post proof for %v", postOut)
			}

			// Proof generation successful, stop retrying
			params.Partitions = partitions
			params.Proofs = postOut
			return false, nil
		}

		// Proof generation failed, so retry

		if len(ps) == 0 {
			// If we didn't skip any new sectors, we failed
			// for some other reason and we need to abort.
			return false, fmt.Errorf("running window post failed: %w", err)
		}
		// TODO: maybe mark these as faulty somewhere?

		alog.Warnw("skipped sectors", "sectors", ps)

		// Explicitly make sure we haven't aborted this PoSt
		// (GenerateWindowPoSt may or may not check this).
		// Otherwise, we could try to continue proving a
		// deadline after the deadline has ended.
		if cerr := ir.ctx.Err(); cerr != nil {
			return false, cerr
		}

		skipCount += uint64(len(ps))
		for _, sector := range ps {
			postSkipped.Set(uint64(sector.Number))
		}

		return true, err
	}

	defer func() {
		ir.proofs.Lock()
		ir.proofs.proofs[batchIdx] = params
		ir.proofs.done++
		ir.proofs.Unlock()
	}()

	pblog := glog.With("batch-idx", batchIdx, "batch-count", len(batch), "partition-start", batchPartitionStartIdx)
	for attempt := 0; ; attempt++ {
		alog := pblog.With("attempt", attempt)
		needRetry, err := proveAttemp(alog)
		if err != nil {
			alog.Errorf("attempt to generate window post proof: %v", err)
		}

		if !needRetry {
			alog.Info("partition batch done")
			break
		}

		select {
		case <-ir.ctx.Done():
			return

		case <-time.After(5 * time.Second):
		}

		alog.Debug("retry partition batch")
	}

}

func (ir *innerRunner) sectorsForProof(goodSectors, allSectors bitfield.BitField) ([]builtin.ExtendedSectorInfo, error) {
	sset, err := ir.pctx.chain.StateMinerSectors(ir.ctx, ir.maddr, &goodSectors, ir.startCtx.ts.Key())
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
func (ir *innerRunner) batchPartitions(partitions []chain.Partition, nv network.Version) ([][]chain.Partition, error) {
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
	partitionsPerMsg, err := specpolicy.GetMaxPoStPartitions(nv, ir.proofType)
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

	if max := int(ir.startCtx.pcfg.MaxPartitionsPerPoStMessage); max > 0 && partitionsPerMsg > max {
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

func (ir *innerRunner) handleFaults(baseLog *logging.ZapLogger) {
	declDeadlineIndex := (ir.dinfo.Index + 2) % ir.dinfo.WPoStPeriodDeadlines
	hflog := baseLog.With("decl-index", declDeadlineIndex)

	tsk := ir.startCtx.ts.Key()

	partitions, err := ir.pctx.chain.StateMinerPartitions(ir.ctx, ir.maddr, declDeadlineIndex, tsk)
	if err != nil {
		hflog.Errorf("get partitions: %v", err)
		return
	}

	if err := ir.checkRecoveries(hflog, declDeadlineIndex, partitions); err != nil {
		// TODO: This is potentially quite bad, but not even trying to post when this fails is objectively worse
		hflog.Errorf("checking sector recoveries: %v", err)
	}

	if ir.startCtx.ts.Height() > policy.NetParams.Network.ForkUpgradeParam.UpgradeIgnitionHeight {
		return // FORK: declaring faults after ignition upgrade makes no sense
	}

	if err := ir.checkFaults(hflog, declDeadlineIndex, partitions); err != nil {
		// TODO: This is also potentially really bad, but we try to post anyways
		hflog.Errorf("checking sector faults: %v", err)
	}

	return
}

func (ir *innerRunner) checkRecoveries(l *logging.ZapLogger, declIndex uint64, partitions []chain.Partition) error {
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

		mid, resCh, err := ir.publishMessage(stbuiltin.MethodsMiner.DeclareFaultsRecovered, params, true, true)
		if err != nil {
			hlog.Errorf("publish message: %s", err)
			return
		}

		hlog = hlog.With("mid", mid)

		hlog.Warn("declare faults recovered message published")

		res := <-resCh

		if res.err != nil {
			hlog.Errorf("declare faults recovered wait error: %s", res.err)
			return
		}

		if res.Message.Receipt.ExitCode != 0 {
			hlog.Errorf("declare faults recovered %s got non-0 exit code: %d", res.Message.SignedCid, res.Message.Receipt.ExitCode)
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

		recovered, err := ir.checkSectors(plog, unrecovered)
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

		recoverTotal += recoveredCount

		currentParams.Recoveries = append(currentParams.Recoveries, miner.RecoveryDeclaration{
			Deadline:  declIndex,
			Partition: uint64(partIdx),
			Sectors:   recovered,
		})

		if max := ir.startCtx.pcfg.MaxPartitionsPerRecoveryMessage; max > 0 &&
			len(currentParams.Recoveries) >= int(max) {

			go handleRecoverMessage(currentParams)
			currentParams = newParams()
		}
	}

	if recoverTotal == 0 && faulty != 0 {
		cklog.Warnw("No recoveries to declare", "faulty", faulty)
	}

	go handleRecoverMessage(currentParams)

	return nil
}

func (ir *innerRunner) checkFaults(l *logging.ZapLogger, declIndex uint64, partitions []chain.Partition) error {
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

		good, err := ir.checkSectors(plog, nonFaulty)
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

	uid, waitCh, err := ir.publishMessage(stbuiltin.MethodsMiner.DeclareFaults, params, true, true)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	cklog.Warnw("declare faults message published", "mid", uid)

	res := <-waitCh

	if res.err != nil {
		return fmt.Errorf("declare faults wait error: %w", res.err)
	}

	if res.Message.Receipt.ExitCode != 0 {
		return fmt.Errorf("declare faults msg %s got non-0 exit code: %d", res.Message.SignedCid, res.Message.Receipt.ExitCode)
	}

	return nil
}

func (ir *innerRunner) checkSectors(clog *logging.ZapLogger, check bitfield.BitField) (bitfield.BitField, error) {
	sectorInfos, err := ir.pctx.chain.StateMinerSectors(ir.ctx, ir.maddr, &check, ir.startCtx.ts.Key())
	if err != nil {
		return bitfield.BitField{}, fmt.Errorf("call StateMinerSectors: %w", err)
	}

	sectors := make(map[abi.SectorNumber]struct{})
	var tocheck []builtin.ExtendedSectorInfo
	for _, info := range sectorInfos {
		sectors[info.SectorNumber] = struct{}{}
		tocheck = append(tocheck, util.SectorOnChainInfoToExtended(info))
	}

	bad, err := ir.sectorTracker.Provable(ir.ctx, ir.mid, tocheck, ir.startCtx.pcfg.StrictCheck)
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

type msgOrErr struct {
	*messager.Message
	err error
}

func (ir *innerRunner) publishMessage(method abi.MethodNum, params cbor.Marshaler, useExtraMsgID bool, wait bool) (string, <-chan msgOrErr, error) {
	encoded, aerr := actors.SerializeParams(params)
	if aerr != nil {
		return "", nil, fmt.Errorf("serialize params: %w", aerr)
	}

	msg := types.Message{
		From:      ir.startCtx.pcfg.Sender.Std(),
		To:        ir.maddr,
		Method:    method,
		Params:    encoded,
		Value:     types.NewInt(0),
		GasFeeCap: ir.startCtx.pcfg.GetGasFeeCap().Std(),
	}

	spec := ir.startCtx.pcfg.FeeConfig.GetSendSpec()

	mid := ""
	if !useExtraMsgID {
		mid = msg.Cid().String()
	} else {
		mid = fmt.Sprintf("%s-%v-%v", msg.Cid().String(), ir.dinfo.Index, ir.dinfo.Open)
	}

	uid, err := ir.pctx.msg.PushMessageWithId(ir.ctx, mid, &msg, &spec)
	if err != nil {
		return "", nil, fmt.Errorf("push msg with id %s: %w", mid, err)
	}

	if !wait {
		return uid, nil, nil
	}

	ch := make(chan msgOrErr, 1)
	go func() {
		defer close(ch)

		m, err := ir.waitMessage(uid, ir.startCtx.pcfg.Confidence)
		ch <- msgOrErr{
			Message: m,
			err:     err,
		}
	}()

	return uid, ch, nil
}

func (ir *innerRunner) waitMessage(mid string, confidence uint64) (*messager.Message, error) {
	return ir.pctx.msg.WaitMessage(ir.ctx, mid, confidence)
}

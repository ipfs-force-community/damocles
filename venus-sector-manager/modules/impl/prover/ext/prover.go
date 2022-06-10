package ext

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/venus/venus-shared/actors/builtin"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/core"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/modules/impl/prover"
	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

var _ core.Prover = (*Prover)(nil)

var log = logging.New("porver-ext")

type reqWithChan struct {
	req    Request
	respCh chan Response
}

func (rwc *reqWithChan) response(ctx context.Context, resp Response) {
	select {
	case <-ctx.Done():

	case rwc.respCh <- resp:
	}
}

func newSubProcessor(ctx context.Context, name string, cfg ProcessorConfig) (*subProcessor, error) {
	bin := os.Args[0]
	args := []string{"processor", name}

	if cfg.Bin != nil {
		bin = *cfg.Bin
		args = cfg.Args
	}

	env := os.Environ()
	for key, val := range cfg.Envs {
		env = append(env, fmt.Sprintf("%s=%s", key, val))
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("set stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("set stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("start cmd: %w", err)
	}

	concurrentLimit := int(cfg.Concurrent)
	if concurrentLimit == 0 {
		concurrentLimit = 1
	}

	return &subProcessor{
		cfg:     cfg,
		limiter: make(chan struct{}, concurrentLimit),
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,

		reqCh:  make(chan reqWithChan, 1),
		respCh: make(chan Response, 1),
		reqSeq: 0,
	}, nil

}

type subProcessor struct {
	cfg     ProcessorConfig
	limiter chan struct{}
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser

	startOnce sync.Once

	reqCh  chan reqWithChan
	respCh chan Response
	reqSeq uint64
}

func (sp *subProcessor) start(ctx context.Context) {
	sp.startOnce.Do(func() {
		go sp.handleRequest(ctx)
		go sp.handleResponse(ctx)
	})
}

func (sp *subProcessor) stop() {
	_ = sp.cmd.Process.Kill()
	_ = sp.cmd.Wait()
	sp.stdin.Close()
	sp.stdout.Close()
}

func (sp *subProcessor) handleResponse(ctx context.Context) {
	splog := log.With("pid", sp.cmd.Process.Pid, "ppid", os.Getpid(), "loop", "resp")
	splog.Info("response loop start")
	defer splog.Info("response loop stop")

	reader := bufio.NewReaderSize(sp.stdout, 1<<20)
	var readErr error

	for {
		b, err := reader.ReadBytes('\n')
		if err != nil {
			readErr = err
			break
		}

		var resp Response
		err = json.Unmarshal(b, &resp)
		if err != nil {
			splog.Warnf("decode response from stdout of sub: %s", err)
			continue
		}

		splog.Debugw("response received", "id", resp.ID, "bytes", len(b))

		select {
		case <-ctx.Done():
			return

		case sp.respCh <- resp:
			splog.Debugw("response sent", "id", resp.ID)
		}
	}

	if readErr != nil {
		splog.Warnf("sub stdout scanner error: %s", readErr)
	}

}

func (sp *subProcessor) handleRequest(ctx context.Context) {
	splog := log.With("pid", sp.cmd.Process.Pid, "ppid", os.Getpid(), "loop", "req")
	splog.Info("request loop start")
	defer splog.Info("request loop stop")

	requests := map[uint64]reqWithChan{}
	writer := bufio.NewWriter(sp.stdin)

LOOP:
	for {
		select {
		case <-ctx.Done():
			splog.Debug("context done")
			return

		case req := <-sp.reqCh:
			splog.Debugw("request received", "id", req.req.ID)
			n, err := WriteData(writer, req.req)
			if err != nil {
				errMsg := err.Error()
				req.response(ctx, Response{
					ID:     req.req.ID,
					ErrMsg: &errMsg,
					Result: nil,
				})
				continue LOOP
			}

			splog.Debugw("request sent", "id", req.req.ID, "bytes", n)
			requests[req.req.ID] = req

		case resp := <-sp.respCh:
			req, ok := requests[resp.ID]
			if !ok {
				splog.Warnw("request not found", "id", resp.ID)
				continue LOOP
			}

			req.response(ctx, resp)
		}
	}
}

func (sp *subProcessor) process(ctx context.Context, data interface{}, res interface{}) error {
	req := Request{
		ID: atomic.AddUint64(&sp.reqSeq, 1),
	}

	if err := req.SetData(data); err != nil {
		return fmt.Errorf("set data in request: %w", err)
	}

	select {
	case sp.limiter <- struct{}{}:

	case <-ctx.Done():
		return ctx.Err()
	}

	defer func() {
		<-sp.limiter
	}()

	respCh := make(chan Response, 1)
	select {
	case <-ctx.Done():
		return ctx.Err()

	case sp.reqCh <- reqWithChan{
		req:    req,
		respCh: respCh,
	}:

	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case resp := <-respCh:
		if resp.ErrMsg != nil {
			return fmt.Errorf(*resp.ErrMsg)
		}

		if res != nil {
			err := resp.DecodeInto(res)
			if err != nil {
				return fmt.Errorf("decode into result: %w", err)
			}
		}

		return nil
	}
}

func New(ctx context.Context, procCfgs []ProcessorConfig) (*Prover, error) {
	ctx, cancel := context.WithCancel(ctx)

	subs := make([]*subProcessor, 0, len(procCfgs))
	for pi := range procCfgs {
		sub, err := newSubProcessor(ctx, ProcessorNameWindostPoSt, procCfgs[pi])
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start sub processor #%d: %w", pi, err)
		}

		subs = append(subs, sub)
	}

	return &Prover{
		ctx:    ctx,
		cancel: cancel,
		subs:   subs,
	}, nil
}

type Prover struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	subs   []*subProcessor
}

func (p *Prover) Run() {
	for si := range p.subs {
		p.subs[si].start(p.ctx)
	}
}

func (p *Prover) Close() {
	p.cancel()
	for si := range p.subs {
		p.subs[si].stop()
	}
}

func (*Prover) AggregateSealProofs(ctx context.Context, aggregateInfo core.AggregateSealVerifyProofAndInfos, proofs [][]byte) ([]byte, error) {
	return prover.Prover.AggregateSealProofs(ctx, aggregateInfo, proofs)
}

func (p *Prover) GenerateWindowPoSt(ctx context.Context, minerID abi.ActorID, sectors prover.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, []abi.SectorID, error) {
	sub, ok, err := p.findCandidateProcessor()
	if err != nil {
		return nil, nil, fmt.Errorf("find candidate sub processor: %w", err)
	}

	if !ok {
		return nil, nil, fmt.Errorf("no available sub processor")
	}

	var res WindowPoStResult

	err = sub.process(ctx, WindowPoStData{
		Miner:      minerID,
		Sectors:    sectors,
		Randomness: randomness,
	}, &res)
	if err != nil {
		return nil, nil, fmt.Errorf("sub process: %w", err)
	}

	return res.Proof, res.Skipped, nil
}

func (*Prover) GenerateWinningPoSt(ctx context.Context, minerID abi.ActorID, sectors prover.SortedPrivateSectorInfo, randomness abi.PoStRandomness) ([]builtin.PoStProof, error) {
	return prover.Prover.GenerateWinningPoSt(ctx, minerID, sectors, randomness)
}

func (p *Prover) findCandidateProcessor() (*subProcessor, bool, error) {
	if len(p.subs) == 0 {
		return nil, false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var available []*subProcessor
	for _, sub := range p.subs {
		if cap(sub.limiter) > len(sub.limiter) {
			available = append(available, sub)
		}
	}

	if len(available) == 0 {
		available = p.subs
	}

	var totalWeight int
	weights := make([]int, len(available))

	for i, sub := range available {
		weight := int(sub.cfg.Weight)
		if weight == 0 {
			weight = 1
		}

		totalWeight += weight
		weights[i] = totalWeight
	}

	choice := rand.Intn(totalWeight)
	chosenIdx := 0
	for i := 0; i < len(weights)-1; i++ {
		if choice < weights[i+1] {
			chosenIdx = i
			break
		}
	}

	log.Debugw("sub processor selected", "candidates", len(available), "weights-max", totalWeight, "weight-selected", choice, "candidate-index", chosenIdx)

	return available[chosenIdx], true, nil
}

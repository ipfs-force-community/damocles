package extproc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ipfs-force-community/venus-cluster/venus-sector-manager/pkg/logging"
)

const VenusWorkerBin = "venus-worker"

var log = logging.New("extproc")

func New(ctx context.Context, taskName string, cfgs []ExtProcessorConfig) (*Processor, error) {
	ctx, cancel := context.WithCancel(ctx)

	exts := make([]*ExtProcessor, 0, len(cfgs))
	for ci := range cfgs {
		ext, err := newExtProcessor(ctx, taskName, cfgs[ci])
		if err != nil {
			cancel()
			return nil, fmt.Errorf("start ext processor #%d: %w", ci, err)
		}

		exts = append(exts, ext)
	}

	return &Processor{
		ctx:    ctx,
		cancel: cancel,
		exts:   exts,
	}, nil
}

type Processor struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	exts   []*ExtProcessor
}

func (p *Processor) findCandidateExt() (*ExtProcessor, bool, error) {
	if len(p.exts) == 0 {
		return nil, false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	var available []*ExtProcessor
	for _, ext := range p.exts {
		if cap(ext.limiter) > len(ext.limiter) {
			available = append(available, ext)
		}
	}

	if len(available) == 0 {
		available = p.exts
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

	log.Debugw("ext processor selected", "candidates", len(available), "weights-max", totalWeight, "weight-selected", choice, "candidate-index", chosenIdx)

	return available[chosenIdx], true, nil
}

func (p *Processor) Process(ctx context.Context, data interface{}, res interface{}) error {
	ext, ok, err := p.findCandidateExt()
	if err != nil {
		return fmt.Errorf("find candidate sub processor: %w", err)
	}

	if !ok {
		return fmt.Errorf("no available sub processor")
	}

	err = ext.process(ctx, data, res)
	if err != nil {
		return fmt.Errorf("ext process: %w", err)
	}

	return nil
}

func (p *Processor) Run() {
	for si := range p.exts {
		p.exts[si].start(p.ctx)
	}
}

func (p *Processor) Close() {
	p.cancel()
	for si := range p.exts {
		p.exts[si].stop()
	}
}

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

func waitForReady(ctx context.Context, reader *bufio.Reader, taskName string, timeout time.Duration) error {
	ready := make(chan error, 1)
	go func() {
		defer close(ready)
		msg, err := reader.ReadString('\n')
		if err != nil {
			ready <- fmt.Errorf("read ready message: %w", err)
			return
		}

		if msg != ReadyMessage(taskName) {
			ready <- fmt.Errorf("unexpected ready message %q", msg)
			return
		}

	}()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case err := <-ready:
		return err

	case <-time.After(timeout):
		return fmt.Errorf("wait for ready message timeout")
	}
}

func newExtProcessor(ctx context.Context, taskName string, cfg ExtProcessorConfig) (*ExtProcessor, error) {
	bin := filepath.Join(filepath.Dir(os.Args[0]), VenusWorkerBin)
	args := []string{"processor", taskName}

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

	stdwriter := bufio.NewWriterSize(stdin, 1<<20)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("set stdout pipe: %w", err)
	}

	stdreader := bufio.NewReaderSize(stdout, 1<<20)

	cmd.Stderr = os.Stderr

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("start cmd: %w", err)
	}

	readySecs := cfg.ReadyTimeoutSecs
	if readySecs == 0 {
		readySecs = 5
	}

	err = waitForReady(ctx, stdreader, taskName, time.Duration(readySecs)*time.Second)
	if err != nil {
		cmd.Process.Kill()
		return nil, fmt.Errorf("wait for ready: %w", err)
	}

	concurrentLimit := int(cfg.Concurrent)
	if concurrentLimit == 0 {
		concurrentLimit = 1
	}

	return &ExtProcessor{
		taskName: taskName,
		cfg:      cfg,
		limiter:  make(chan struct{}, concurrentLimit),
		cmd:      cmd,

		rawStdIn:  stdin,
		rawStdOut: stdout,
		stdwriter: stdwriter,
		stdreader: stdreader,

		reqCh:  make(chan reqWithChan, 1),
		respCh: make(chan Response, 1),
		reqSeq: 0,
	}, nil

}

type ExtProcessor struct {
	taskName string
	cfg      ExtProcessorConfig
	limiter  chan struct{}
	cmd      *exec.Cmd

	rawStdIn  io.WriteCloser
	rawStdOut io.ReadCloser

	stdwriter *bufio.Writer
	stdreader *bufio.Reader

	startOnce sync.Once

	reqCh  chan reqWithChan
	respCh chan Response
	reqSeq uint64
}

func (ep *ExtProcessor) start(ctx context.Context) {
	ep.startOnce.Do(func() {
		go ep.handleRequest(ctx)
		go ep.handleResponse(ctx)
	})
}

func (ep *ExtProcessor) stop() {
	_ = ep.cmd.Process.Kill()
	_ = ep.cmd.Wait()
	ep.rawStdIn.Close()
	ep.rawStdOut.Close()
}

func (ep *ExtProcessor) handleResponse(ctx context.Context) {
	splog := log.With("pid", ep.cmd.Process.Pid, "ppid", os.Getpid(), "task", ep.taskName, "loop", "resp")
	splog.Info("response loop start")
	defer splog.Info("response loop stop")

	reader := ep.stdreader
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

		case ep.respCh <- resp:
			splog.Debugw("response sent", "id", resp.ID)
		}
	}

	splog.Warnf("sub stdout scanner error: %s", readErr)
}

func (ep *ExtProcessor) handleRequest(ctx context.Context) {
	splog := log.With("pid", ep.cmd.Process.Pid, "ppid", os.Getpid(), "task", ep.taskName, "loop", "req")
	splog.Info("request loop start")
	defer splog.Info("request loop stop")

	requests := map[uint64]reqWithChan{}
	writer := ep.stdwriter

LOOP:
	for {
		select {
		case <-ctx.Done():
			splog.Debug("context done")
			return

		case req := <-ep.reqCh:
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

		case resp := <-ep.respCh:
			req, ok := requests[resp.ID]
			if !ok {
				splog.Warnw("request not found", "id", resp.ID)
				continue LOOP
			}

			req.response(ctx, resp)
		}
	}
}

func (ep *ExtProcessor) process(ctx context.Context, data interface{}, res interface{}) error {
	req := Request{
		ID: atomic.AddUint64(&ep.reqSeq, 1),
	}

	if err := req.SetData(data); err != nil {
		return fmt.Errorf("set data in request: %w", err)
	}

	select {
	case ep.limiter <- struct{}{}:

	case <-ctx.Done():
		return ctx.Err()
	}

	defer func() {
		<-ep.limiter
	}()

	respCh := make(chan Response, 1)
	select {
	case <-ctx.Done():
		return ctx.Err()

	case ep.reqCh <- reqWithChan{
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

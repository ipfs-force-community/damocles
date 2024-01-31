package poster

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs-force-community/damocles/damocles-manager/pkg/logging"
)

var DefaultRetryNum = 5

type EventType string
type EventBody struct {
	Type EventType
	Data any
}
type Event *EventBody

// State indicate the stage of wdpost task
type State = string

const (
	StateEmpty State = ""
	StateAbort State = "abort"
	StateAll   State = "*"
)

var (
	EventIdle Event
)

type ApplyResult struct {
	state State
	event Event
	err   error
}

type Scheduler interface {
	State() State
	Apply(Event)
	Abort(e error)
}

type Machine struct {
	logger       *logging.ZapLogger
	currentState State
	preState     State

	errRetry int

	eventChan      chan Event
	innerEventChan chan Event
	errorTrace     []error

	handler map[State]map[EventType]func(data any) (State, Event, error)

	ctx    context.Context
	cancel context.CancelFunc

	// indicate the state machine finish
	done chan struct{}

	mux sync.RWMutex
}

var _ Scheduler = (*Machine)(nil)

func (m *Machine) Start(ctx context.Context) {
	if m.logger == nil {
		m.logger = log.With("module", "state_machine")
	}
	if m.errRetry == 0 {
		m.errRetry = DefaultRetryNum
	}
	if m.eventChan == nil {
		m.eventChan = make(chan Event, 1024)
	}
	if m.innerEventChan == nil {
		m.innerEventChan = make(chan Event, 1024)
	}
	if m.done == nil {
		m.done = make(chan struct{})
	}
	m.ctx, m.cancel = context.WithCancel(ctx)

	errorCounter := 0
	applyEvent := func(e Event) {
		s, e, err := m.handle(e)
		if err != nil {
			errorCounter++
			m.errorTrace = append(m.errorTrace, err)
		} else {
			errorCounter = 0
		}

		if errorCounter > m.errRetry {
			m.errorTrace = append(m.errorTrace, fmt.Errorf("error retry exceed %d/%d", errorCounter, m.errRetry))
			s = StateAbort
		}

		if s != StateEmpty {
			m.updateState(s)
		}

		if e != EventIdle {
			m.innerEventChan <- e
		}
	}

	go func() {
	EVENT_LOOP:
		for {
			if m.currentState == StateAbort {
				errorTraceFmtCache := ""
				for _, err := range m.errorTrace {
					errorTraceFmtCache += fmt.Sprintf("	* %s \n ", err)
				}
				m.Abort(fmt.Errorf("inner abort with trace: \n %s", errorTraceFmtCache))
				break
			}

			select {
			case <-m.ctx.Done():
				m.errorTrace = append(m.errorTrace, m.ctx.Err())
				m.logger.Info("context done")
				break EVENT_LOOP
			case event := <-m.innerEventChan:
				applyEvent(event)
			case event := <-m.eventChan:
				applyEvent(event)
			}
		}
		close(m.done)
	}()
}

func (m *Machine) Apply(e Event) {
	select {
	case <-m.done:
		// ignore event after done
	default:
		// drop when event chan is full
		if len(m.eventChan) < cap(m.eventChan) {
			m.eventChan <- e
		}
	}
}

func (m *Machine) State() State {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.currentState
}

func (m *Machine) Abort(e error) {
	m.cancel()
	m.logger.Errorf("abort with error: %s", e)
}

func (m *Machine) Wait() (State, []error) {
	<-m.done
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.currentState, m.errorTrace
}

func (m *Machine) RegisterHandler(s State, e EventType, handler func(data any) (State, Event, error)) {
	if m.handler == nil {
		m.handler = make(map[State]map[EventType]func(data any) (State, Event, error))
	}

	if _, ok := m.handler[s]; !ok {
		m.handler[s] = make(map[EventType]func(data any) (State, Event, error))
	}

	m.handler[s][e] = handler
}

func (m *Machine) handle(e Event) (State, Event, error) {
	if handler, ok := m.handler[StateAll]; ok {
		if f, ok := handler[e.Type]; ok {
			return f(e.Data)
		}
	}

	handler, ok := m.handler[m.currentState]
	if !ok {
		return "", EventIdle, nil
	}

	f, ok := handler[e.Type]
	if !ok {
		return "", EventIdle, nil
	}

	return f(e.Data)
}

func (m *Machine) updateState(s State) {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.preState = m.currentState
	m.currentState = s
	m.logger.Debugf("state transfer: %s -> %s", m.preState, m.currentState)
}

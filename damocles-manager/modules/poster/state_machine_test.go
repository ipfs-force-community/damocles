package poster

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStateTransfer(t *testing.T) {

	ctx := context.Background()
	m := &Machine{
		errRetry: 1,
	}
	require.Equal(t, StateEmpty, m.State())

	m.RegisterHandler(StateEmpty, "tick", func(data any) (State, Event, error) {
		return "A", nil, nil
	})
	m.RegisterHandler("A", "tick", func(data any) (State, Event, error) {
		e := &EventBody{Type: "toB"}
		return "", e, nil
	})
	m.RegisterHandler("A", "toB", func(data any) (State, Event, error) {
		return "B", nil, nil
	})
	m.RegisterHandler("B", "tick", func(data any) (State, Event, error) {
		e := &EventBody{Type: "toC"}
		return "interval", e, nil
	})
	m.RegisterHandler("interval", "toC", func(data any) (State, Event, error) {
		return "C", nil, nil
	})
	m.RegisterHandler("C", "tick", func(data any) (State, Event, error) {
		return "", nil, fmt.Errorf("should abort after two error")
	})
	m.Start(ctx)

	cases := []struct {
		title        string
		eventToApply Event
		stateExpect  State
	}{
		{
			title:        "ignore unconcerned event",
			eventToApply: &EventBody{Type: "unknown"},
			stateExpect:  StateEmpty,
		},
		{
			title:        "transfer by result.state",
			eventToApply: &EventBody{Type: "tick"},
			stateExpect:  "A",
		},
		{
			title:        "transfer by result.event",
			eventToApply: &EventBody{Type: "tick"},
			stateExpect:  "B",
		},
		{
			title:        "transfer by result.state and result.event",
			eventToApply: &EventBody{Type: "tick"},
			stateExpect:  "C",
		},
		{
			title:        "1st error",
			eventToApply: &EventBody{Type: "tick"},
			stateExpect:  "C",
		},
		{
			title:        "2nd error ",
			eventToApply: &EventBody{Type: "tick"},
			stateExpect:  StateAbort,
		},
	}

	for _, c := range cases {
		m.Apply(c.eventToApply)

		// make sure exec adequately
		time.Sleep(100 * time.Millisecond)

		require.Equalf(t, c.stateExpect, m.State(), c.title)
	}

	m.Wait()
}

func TestAbort(t *testing.T) {
	ctx := context.Background()
	m := &Machine{}
	require.Equal(t, StateEmpty, m.State())

	m.RegisterHandler(StateEmpty, "tick", func(data any) (State, Event, error) {
		time.Sleep(100 * time.Millisecond)
		fmt.Println("tick")
		return "", &EventBody{Type: "tick"}, nil
	})
	m.Start(ctx)

	m.Apply(&EventBody{Type: "tick"})
	m.Abort(fmt.Errorf("abort manually"))
	m.Wait()
}

func TestApply(t *testing.T) {
	m := &Machine{
		eventChan: make(chan Event, 1),
	}

	m.RegisterHandler(StateEmpty, "tick", func(data any) (State, Event, error) {
		// simulate long time task
		time.Sleep(1 * time.Hour)
		return "", nil, nil
	})

	m.Apply(&EventBody{Type: "tick"})
	m.Apply(&EventBody{Type: "tick"})
	m.Apply(&EventBody{Type: "tick"})
	m.Apply(&EventBody{Type: "tick"})

	require.Equal(t, 1, len(m.eventChan))
}

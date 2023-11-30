package main_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	limiter "gitlab.forceup.in/ForceMining/force-ext-processors/vsm-plugins/concurrent-limiter"
)

func TestConcurrentLimiter_Acquire(t *testing.T) {
	cl := testConcurrentLimiter(t, map[string]uint16{"add-pieces": 1})
	slot, err := cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot)

	noSlot, err := cl.Acquire("add-pieces", "b")
	require.NoError(t, err)
	require.Empty(t, noSlot)

	time.Sleep(time.Second)
	slot, err = cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot)
}

func TestConcurrentLimiter_Acquire2(t *testing.T) {
	cl := testConcurrentLimiter(t, map[string]uint16{"add-pieces": 2})
	slot1, err := cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot1)

	slot2, err := cl.Acquire("add-pieces", "b")
	require.NoError(t, err)
	require.NotEmpty(t, slot2)

	require.NotEqual(t, slot1, slot2)
}

func TestConcurrentLimiter_Release(t *testing.T) {
	cl := testConcurrentLimiter(t, map[string]uint16{"add-pieces": 1})
	slot, err := cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot)

	noSlot, err := cl.Acquire("add-pieces", "b")
	require.NoError(t, err)
	require.Empty(t, noSlot)

	require.NoError(t, cl.Release(slot, "a"))

	slot, err = cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot)
}

func TestConcurrentLimiter_Extend(t *testing.T) {
	cl := testConcurrentLimiter(t, map[string]uint16{"add-pieces": 1})
	slot, err := cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot)

	time.Sleep(500 * time.Millisecond)

	ok, err := cl.Extend(slot, "a")
	require.NoError(t, err)
	require.True(t, ok)

	time.Sleep(500 * time.Millisecond)

	noSlot, err := cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.Empty(t, noSlot)
}

func TestConcurrentLimiter_ExtendWithInvalidId(t *testing.T) {
	cl := testConcurrentLimiter(t, map[string]uint16{"add-pieces": 2})
	slot, err := cl.Acquire("add-pieces", "a")
	require.NoError(t, err)
	require.NotEmpty(t, slot)

	ok, err := cl.Extend(slot, "b")
	require.NoError(t, err)
	require.False(t, ok)
}

func testConcurrentLimiter(t *testing.T, concurrent map[string]uint16) *limiter.ConcurrentLimiter {
	return limiter.NewConcurrentLimiter(concurrent, time.Second, testStore(t))
}

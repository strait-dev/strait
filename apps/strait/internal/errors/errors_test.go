package errors

import (
	"fmt"
	"testing"

	"github.com/sourcegraph/conc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSentinel = fmt.Errorf("sentinel")

type myErr struct{ Code int }

func (e *myErr) Error() string { return "my error" }

func TestWrap_PreservesErrorsIs(t *testing.T) {
	t.Parallel()

	wrapped := Wrap(errSentinel, "context")

	assert.ErrorIs(t, wrapped, errSentinel)
}

func TestWrap_PreservesErrorsAs(t *testing.T) {
	t.Parallel()

	original := &myErr{Code: 42}
	wrapped := Wrap(original, "context")
	var target *myErr

	require.ErrorAs(t, wrapped, &target)
	assert.Equal(t, 42, target.Code)
}

func TestWrap_AddsMessage(t *testing.T) {
	t.Parallel()

	wrapped := Wrap(errSentinel, "operation failed")

	assert.Contains(t, wrapped.Error(), "operation failed")
}

func TestWrap_NilError(t *testing.T) {
	t.Parallel()

	assert.NoError(t, Wrap(nil, "msg"))
}

func TestWrapf_FormatsMessage(t *testing.T) {
	t.Parallel()

	wrapped := Wrapf(errSentinel, "failed %s %d", "op", 42)

	assert.Contains(t, wrapped.Error(), "failed op 42")
}

func TestIn_SetsComponent(t *testing.T) {
	t.Parallel()

	err := In("worker").Wrap(errSentinel)

	require.Error(t, err)
}

func TestIn_WithAttributes(t *testing.T) {
	t.Parallel()

	err := In("worker").With("run_id", "r-1").Wrap(errSentinel)

	require.Error(t, err)
	assert.ErrorIs(t, err, errSentinel)
}

func TestIn_ChainedAttributes(t *testing.T) {
	t.Parallel()

	err := In("scheduler").With("job_id", "j-1").With("attempt", 3).Wrap(errSentinel)

	require.Error(t, err)
}

func TestNew_ErrorString(t *testing.T) {
	t.Parallel()

	err := New("something broke")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "something broke")
}

func TestWrap_DeepNesting(t *testing.T) {
	t.Parallel()

	base := errSentinel
	err := Wrap(Wrap(Wrap(base, "layer1"), "layer2"), "layer3")

	assert.ErrorIs(t, err, errSentinel)
}

func TestWrapf_NilError(t *testing.T) {
	t.Parallel()

	assert.NoError(t, Wrapf(nil, "format %s %d", "arg", 1))
}

func TestIn_ConcurrentUsage(t *testing.T) {
	t.Parallel()
	var wg conc.WaitGroup
	for range 50 {
		wg.Go(func() {
			_ = In("worker").With("id", "x").Wrap(errSentinel)
		})
	}
	wg.Wait()
}

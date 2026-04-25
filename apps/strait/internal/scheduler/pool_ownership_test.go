package scheduler

import (
	"context"
	"testing"
)

type fakePartitionTunerStore struct{}

func (fakePartitionTunerStore) ListJobRunsPartitions(context.Context) ([]string, error) {
	return nil, nil
}

func (fakePartitionTunerStore) ExecDDL(context.Context, string) error {
	return nil
}

func (fakePartitionTunerStore) PartitionExists(context.Context, string) (bool, error) {
	return true, nil
}

func TestPartitionTuner_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	tuner := NewPartitionTuner(fakePartitionTunerStore{}, PartitionTunerConfig{})
	tuner.Close()
	tuner.Close()
}

func TestDLQAgeOut_CloseIsIdempotent(t *testing.T) {
	t.Parallel()

	archiver := NewDLQAgeOut(&fakeDLQAgeOutStore{}, DLQAgeOutConfig{})
	archiver.Close()
	archiver.Close()
}

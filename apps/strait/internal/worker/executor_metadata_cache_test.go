package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

func TestDispatchSecretsUsesExecutorCacheAcrossContexts(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	store := &mockExecutorStore{
		listSecretsFn: func(_ context.Context, _, _ string) ([]domain.JobSecret, error) {
			calls.Add(1)
			return []domain.JobSecret{{SecretKey: "TOKEN", Value: "secret"}}, nil
		},
	}
	exec := NewExecutor(ExecutorConfig{Store: store})
	job := &domain.Job{ID: "job-1", EnvironmentID: "env-1"}

	first, err := exec.dispatchSecrets(context.Background(), job)
	require.NoError(t, err)
	second, err := exec.dispatchSecrets(context.Background(), job)
	require.NoError(t, err)

	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, first, second)

	second[0].Value = "mutated"
	third, err := exec.dispatchSecrets(context.Background(), job)
	require.NoError(t, err)
	require.Equal(t, "secret", third[0].Value)
}

func TestExecutorMetadataCacheSingleflightsConcurrentMisses(t *testing.T) {
	t.Parallel()

	cache := newExecutorMetadataCache(5*time.Second, cloneJobSecrets)
	var calls atomic.Int64
	var wg sync.WaitGroup
	errCh := make(chan error, 32)
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cache.Load(context.Background(), "secrets:job-1:env-1", func(context.Context) ([]domain.JobSecret, error) {
				calls.Add(1)
				time.Sleep(10 * time.Millisecond)
				return []domain.JobSecret{{SecretKey: "TOKEN", Value: "secret"}}, nil
			})
			if err != nil {
				errCh <- err
				return
			}
			if len(got) != 1 {
				errCh <- context.Canceled
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	require.Equal(t, int64(1), calls.Load())
}

func TestExecutorMetadataCacheExpires(t *testing.T) {
	t.Parallel()

	cache := newExecutorMetadataCache(10*time.Millisecond, cloneJobSecrets)
	var calls atomic.Int64
	load := func(context.Context) ([]domain.JobSecret, error) {
		calls.Add(1)
		return []domain.JobSecret{{SecretKey: "TOKEN", Value: "secret"}}, nil
	}

	_, err := cache.Load(context.Background(), "secrets:job-1:env-1", load)
	require.NoError(t, err)
	time.Sleep(20 * time.Millisecond)
	_, err = cache.Load(context.Background(), "secrets:job-1:env-1", load)
	require.NoError(t, err)

	require.Equal(t, int64(2), calls.Load())
}

func TestCloneWebhookSubscriptionsCopiesEventTypes(t *testing.T) {
	t.Parallel()

	subs := []domain.WebhookSubscription{{
		ID:         "sub-1",
		EventTypes: []string{domain.WebhookEventRunCompleted},
	}}
	cloned := cloneWebhookSubscriptions(subs)
	cloned[0].EventTypes[0] = domain.WebhookEventRunFailed

	require.Equal(t, domain.WebhookEventRunCompleted, subs[0].EventTypes[0])
}

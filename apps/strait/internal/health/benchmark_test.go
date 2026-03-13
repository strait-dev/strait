package health

import (
	"context"
	"strconv"
	"testing"
)

func BenchmarkRegistryCheckAll(b *testing.B) {
	r := NewRegistry()
	for i := range 5 {
		r.Register(NewChecker("noop-"+strconv.Itoa(i), func(context.Context) error {
			return nil
		}))
	}

	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result := r.CheckAll(ctx)
		if result.Status != StatusUp {
			b.Fatalf("CheckAll() status = %q", result.Status)
		}
	}
}

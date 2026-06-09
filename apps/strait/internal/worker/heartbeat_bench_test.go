package worker

import (
	"fmt"
	"testing"
	"time"
)

func newBenchmarkHeartbeatManager(size int) *HeartbeatManager {
	h := NewHeartbeatManager(&mockHeartbeatStore{}, time.Hour)
	for i := range size {
		h.Register(fmt.Sprintf("run-%05d", i))
	}
	return h
}

func BenchmarkHeartbeatManagerActiveCount(b *testing.B) {
	for _, size := range []int{16, 256, 4096} {
		b.Run(fmt.Sprintf("active_%d", size), func(b *testing.B) {
			h := newBenchmarkHeartbeatManager(size)

			b.ReportAllocs()
			for b.Loop() {
				if h.ActiveCount() != size {
					b.Fatal("unexpected active count")
				}
			}
		})
	}
}

func BenchmarkHeartbeatManagerCollectActiveIDs(b *testing.B) {
	for _, size := range []int{16, 256, 4096} {
		b.Run(fmt.Sprintf("active_%d", size), func(b *testing.B) {
			h := newBenchmarkHeartbeatManager(size)

			b.ReportAllocs()
			for b.Loop() {
				ids := h.collectActiveIDs()
				if len(ids) != size {
					b.Fatal("unexpected active ids")
				}
			}
		})
	}
}

package metrics

import (
	"sync/atomic"
	"testing"
	"time"
)

// BenchmarkRecordParallel measures the per-request recording path under
// contention from many goroutines — the load-test hot path.
func BenchmarkRecordParallel(b *testing.B) {
	m := New()
	var i int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddInt64(&i, 1)
			m.Record(int(200+n%3), time.Duration(n%50)*time.Millisecond, n%10 == 0)
		}
	})
}

// BenchmarkRecordSnapshotMixed simulates the real workload: many workers
// recording while a reader (the SSE stream) snapshots concurrently.
func BenchmarkRecordSnapshotMixed(b *testing.B) {
	m := New()
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(2 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				_ = m.Snapshot()
			}
		}
	}()
	var i int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := atomic.AddInt64(&i, 1)
			m.Record(200, time.Duration(n%50)*time.Millisecond, false)
		}
	})
	b.StopTimer()
	close(stop)
}

// BenchmarkRecordAtSharded models the runner's hot path: each parallel worker
// records into its own stable shard, so histogram locks rarely collide.
func BenchmarkRecordAtSharded(b *testing.B) {
	m := New()
	var next int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		shard := int(atomic.AddInt64(&next, 1))
		var n int64
		for pb.Next() {
			n++
			m.RecordAt(shard, 200, time.Duration(n%50)*time.Millisecond, false)
		}
	})
}

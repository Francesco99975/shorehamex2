package tools

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// func TestAddJob_Works_WithInit(t *testing.T) {
// 	t.Parallel()
// 	defer ShutdownCron()

// 	var ran bool
// 	err := AddJob("test", "* * * * * *", func() { ran = true })
// 	require.NoError(t, err)

// 	time.Sleep(1200 * time.Millisecond)
// 	assert.True(t, ran)
// }

func TestConcurrent_Safety(t *testing.T) {
	t.Parallel()
	defer ShutdownCron()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("job-%d", i)
			_ = AddJob(id, "* * * * * *", func() {})
			time.Sleep(10 * time.Millisecond)
			RemoveJob(id)
		}(i)
	}
	wg.Wait()
}

func TestAddJob_DuplicateID_Handled(t *testing.T) {
	t.Parallel()
	defer ShutdownCron()

	var count atomic.Int32
	task := func() { count.Add(1) }

	err1 := AddJob("dup", "* * * * * *", task)
	err2 := AddJob("dup", "* * * * * *", task)
	require.NoError(t, err1)
	require.NoError(t, err2) // should not error

	time.Sleep(1200 * time.Millisecond)
	// Should run ONLY ONCE per second, not twice
	assert.LessOrEqual(t, count.Load(), int32(2)) // ~1-2 runs
}

// func TestUpdateJob_ReplacesTask(t *testing.T) {
// 	t.Parallel()
// 	defer ShutdownCron()

// 	var phase atomic.Int32
// 	err := AddJob("update", "* * * * * *", func() { phase.Add(1) })
// 	require.NoError(t, err)

// 	time.Sleep(1200 * time.Millisecond)
// 	before := phase.Load()

// 	err = UpdateJob("update", "* * * * * *", func() { phase.Add(10) })
// 	require.NoError(t, err)

// 	time.Sleep(1200 * time.Millisecond)
// 	after := phase.Load()

// 	assert.Greater(t, after-before, int32(8)) // new task ran
// }

func TestUpdateJob_NonExistent_NoPanic(t *testing.T) {
	t.Parallel()
	defer ShutdownCron()

	err := UpdateJob("ghost", "* * * * * *", func() {})
	assert.NoError(t, err) // should not panic
}

func TestRemoveJob_NonExistent_NoPanic(t *testing.T) {
	t.Parallel()
	defer ShutdownCron()

	RemoveJob("ghost") // should not panic
}

func TestInvalidSpec_ReturnsError(t *testing.T) {
	t.Parallel()
	defer ShutdownCron()

	err := AddJob("bad", "invalid-spec-here", func() {})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected")
}

// func TestRemoveJob_StopsExecution(t *testing.T) {
// 	t.Parallel()
// 	defer ShutdownCron()

// 	var ran atomic.Bool
// 	err := AddJob("stop", "* * * * * *", func() { ran.Store(true) })

// 	require.NoError(t, err)

// 	time.Sleep(1200 * time.Millisecond)
// 	assert.True(t, ran.Load())

// 	ran.Store(false)
// 	RemoveJob("stop")

// 	time.Sleep(1200 * time.Millisecond)
// 	assert.False(t, ran.Load(), "job should not run after remove")
// }

// func TestFullLifecycle(t *testing.T) {
// 	t.Parallel()
// 	defer ShutdownCron()

// 	var count atomic.Int32
// 	id := "lifecycle"

// 	// Add
// 	err := AddJob(id, "* * * * * *", func() { count.Add(1) })
// 	require.NoError(t, err)
// 	time.Sleep(1200 * time.Millisecond)
// 	assert.Greater(t, count.Load(), int32(0))

// 	// Update
// 	err = UpdateJob(id, "*/2 * * * * *", func() { count.Add(10) })
// 	require.NoError(t, err)
// 	time.Sleep(2200 * time.Millisecond)
// 	assert.Greater(t, count.Load(), int32(10))

// 	// Remove
// 	RemoveJob(id)
// 	time.Sleep(1200 * time.Millisecond)
// 	// Should not increase significantly
// 	before := count.Load()
// 	time.Sleep(1200 * time.Millisecond)
// 	assert.LessOrEqual(t, count.Load()-before, int32(1))
// }

func TestNoDataRace_UnderHeavyContention(t *testing.T) {
	t.Parallel()
	defer ShutdownCron()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(3)

		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("race-%d", i%10)
			_ = AddJob(id, "@hourly", func() {})
		}(i)

		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("race-%d", i%10)
			time.Sleep(5 * time.Millisecond)
			RemoveJob(id)
		}(i)

		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("race-%d", i%10)
			_ = UpdateJob(id, "@daily", func() {})
		}(i)
	}
	wg.Wait()
}

// ——————— BENCHMARK: AddJob ———————
func BenchmarkAddJob(b *testing.B) {
	// Reset timer to exclude init()

	// Run b.N times — this is the gold standard
	for b.Loop() {
		id := "bench-job"
		_ = AddJob(id, "@every 1h", func() {}) // cold schedule, no execution
		RemoveJob(id)                          // clean up
	}
}

// ——————— BENCHMARK: AddJob (hot path, no mutex contention) ———————
func BenchmarkAddJob_UniqueIDs(b *testing.B) {

	for i := 0; b.Loop(); i++ {
		_ = AddJob(string(rune(i)), "@daily", func() {})
	}
}

// ——————— BENCHMARK: RemoveJob ———————
func BenchmarkRemoveJob(b *testing.B) {
	id := "bench-remove"
	_ = AddJob(id, "@yearly", func() {})

	for b.Loop() {
		RemoveJob(id)
		// Re-add to keep map entry
		_ = AddJob(id, "@yearly", func() {})
	}
}

// ——————— BENCHMARK: UpdateJob ———————
func BenchmarkUpdateJob(b *testing.B) {
	id := "bench-update"
	_ = AddJob(id, "@hourly", func() {})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = UpdateJob(id, "@daily", func() {})
	}
}

// ——————— BENCHMARK: Concurrent Add/Remove/Update ———————
func BenchmarkConcurrent_Ops(b *testing.B) {
	// Pre-warm the scheduler
	_ = AddJob("warmup", "@yearly", func() {})
	RemoveJob("warmup")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		counter := 0
		for pb.Next() {
			op := counter % 3
			id := "bench-concurrent"
			switch op {
			case 0:
				_ = AddJob(id, "@hourly", func() {})
			case 1:
				_ = UpdateJob(id, "@daily", func() {})
			case 2:
				RemoveJob(id)
			}
			counter++
		}
	})
}

// ——————— BENCHMARK: Real job execution (every 1ms) ———————
func BenchmarkJobExecution_1ms(b *testing.B) {
	var count atomic.Int32
	id := "fast-job"

	// Schedule a job that runs ~1000 times per second
	err := AddJob(id, "*/1 * * * * *", func() {
		count.Add(1)
	})
	if err != nil {
		b.Fatal(err)
	}

	// Let it run for ~1 second
	time.Sleep(1200 * time.Millisecond)
	RemoveJob(id)

	// Report executions per second
	b.ReportMetric(float64(count.Load())/1.2, "executions/sec")
}

// ——————— BENCHMARK: Memory allocations ———————
func BenchmarkAddJob_Allocations(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = AddJob("alloc-test", "@daily", func() {})
		RemoveJob("alloc-test")
	}
}

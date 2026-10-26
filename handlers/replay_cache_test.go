package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fdm-backend/detection"
)

func TestReplayCacheCoalescesConcurrentRequests(t *testing.T) {
	cache := newReplayCache()
	var builds atomic.Int32
	var ready, finished sync.WaitGroup
	release := make(chan struct{})
	build := func() ([]byte, error) { builds.Add(1); <-release; return []byte("shared"), nil }
	for index := 0; index < 32; index++ {
		ready.Add(1)
		finished.Add(1)
		go func() {
			defer finished.Done()
			ready.Done()
			body, err := cache.get(context.Background(), "flight", build)
			if err != nil || string(body) != "shared" {
				t.Errorf("shared build: %q %v", body, err)
			}
		}()
	}
	ready.Wait()
	close(release)
	finished.Wait()
	if builds.Load() != 1 {
		t.Fatalf("built %d times", builds.Load())
	}
}

func TestReplayCacheWaiterCancellationDoesNotCancelSharedBuild(t *testing.T) {
	cache := newReplayCache()
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = cache.get(context.Background(), "flight", func() ([]byte, error) {
			close(started)
			<-release
			return []byte("complete"), nil
		})
	}()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, waitErr := cache.get(ctx, "flight", func() ([]byte, error) { return nil, errors.New("duplicate build") })
	if !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("canceled waiter: %v", waitErr)
	}
	close(release)
	<-finished
	body, err := cache.get(context.Background(), "flight", func() ([]byte, error) { return nil, errors.New("unexpected rebuild") })
	if err != nil || string(body) != "complete" {
		t.Fatalf("shared result lost: %q %v", body, err)
	}
}

func TestReplayCacheBoundsExpirationAndFailureRecovery(t *testing.T) {
	cache := newReplayCache()
	cache.maxBytes, cache.maxEntries = 10, 2
	now := time.Unix(1000, 0)
	cache.now = func() time.Time { return now }
	get := func(key, value string) {
		t.Helper()
		if _, err := cache.get(context.Background(), key, func() ([]byte, error) { return []byte(value), nil }); err != nil {
			t.Fatal(err)
		}
	}
	get("a", "aaaa")
	get("b", "bbbb")
	get("a", "unused")
	get("c", "cccc")
	if cache.entries["b"] != nil || cache.entries["a"] == nil || cache.bytes != 8 {
		t.Fatal("LRU/byte bounds violated")
	}
	get("oversize", "01234567890")
	if cache.entries["oversize"] != nil || cache.bytes != 8 {
		t.Fatal("oversized response retained")
	}
	now = now.Add(cache.ttl)
	get("a", "fresh")
	if entry := cache.entries["a"].Value.(cachedReplay); string(entry.body) != "fresh" {
		t.Fatal("expired response returned")
	}
	for index := 0; index < 2; index++ {
		_, err := cache.get(context.Background(), "error", func() ([]byte, error) { return nil, errRecordingBusy })
		if !errors.Is(err, errRecordingBusy) || cache.entries["error"] != nil {
			t.Fatal("error was cached")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected builder panic")
			}
		}()
		_, _ = cache.get(context.Background(), "panic", func() ([]byte, error) { panic("test") })
	}()
	get("panic", "retry")
}

func TestReplayCacheKeyTracksInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.csv")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	options := detection.ReplayOptions{StartRow: 2, EndRow: 10, SampleIntervalMs: 1000, MaxPoints: 10000, RebaseTime: true}
	base, err := replayCacheKey(path, "flight", options)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*detection.ReplayOptions){
		func(o *detection.ReplayOptions) { o.StartRow++ }, func(o *detection.ReplayOptions) { o.EndRow++ },
		func(o *detection.ReplayOptions) { o.SampleIntervalMs++ }, func(o *detection.ReplayOptions) { o.MaxPoints++ },
		func(o *detection.ReplayOptions) { o.RebaseTime = false },
		func(o *detection.ReplayOptions) {
			o.PhaseRuns = []detection.PhaseRun{{Phase: "CLIMB", StartRow: 2, EndRow: 10}}
		},
	} {
		changed := options
		change(&changed)
		key, err := replayCacheKey(path, "flight", changed)
		if err != nil || key == base {
			t.Fatalf("changed options reused key: %+v %v", changed, err)
		}
	}
	if err := os.WriteFile(path, []byte("different contents"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := replayCacheKey(path, "flight", options)
	if err != nil || key == base {
		t.Fatal("source change reused cache key")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := replayCacheKey(path, "flight", options); err == nil {
		t.Fatal("deleted source accepted")
	}
}

func BenchmarkReplayResponseCache(b *testing.B) {
	path := filepath.Join(b.TempDir(), "source.csv")
	file, err := os.Create(path)
	if err != nil {
		b.Fatal(err)
	}
	fmt.Fprintln(file, "Time(sec),Latitude,Longitude,Airspeed")
	for index := 0; index < 10000; index++ {
		fmt.Fprintf(file, "%d,-1.3,36.8,120\n", index)
	}
	if err := file.Close(); err != nil {
		b.Fatal(err)
	}
	options := detection.ReplayOptions{SampleIntervalMs: 1000, MaxPoints: 10000}
	build := func() ([]byte, error) {
		result, err := detection.BuildReplay(path, options)
		if err != nil {
			return nil, err
		}
		return json.Marshal(map[string]any{"success": true, "data": result})
	}
	for _, cached := range []bool{false, true} {
		b.Run(fmt.Sprintf("cached=%t", cached), func(b *testing.B) {
			cache := newReplayCache()
			if !cached {
				cache.maxBytes = 0
			}
			key, err := replayCacheKey(path, "flight", options)
			if err != nil {
				b.Fatal(err)
			}
			if _, err = cache.get(context.Background(), key, build); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				key, err := replayCacheKey(path, "flight", options)
				if err != nil {
					b.Fatal(err)
				}
				if _, err = cache.get(context.Background(), key, build); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

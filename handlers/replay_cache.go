package handlers

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"fdm-backend/detection"
)

type cachedReplay struct {
	key     string
	body    []byte
	expires time.Time
}

type pendingReplay struct {
	done chan struct{}
	body []byte
	err  error
}

// Cache serialized, immutable responses rather than sharing mutable replay
// slices. Limits apply to retained payload bytes, entries, and lifetime.
type replayCache struct {
	mu         sync.Mutex
	entries    map[string]*list.Element
	lru        *list.List
	pending    map[string]*pendingReplay
	bytes      int
	maxBytes   int
	maxEntries int
	ttl        time.Duration
	now        func() time.Time
}

func newReplayCache() *replayCache {
	return &replayCache{
		entries: make(map[string]*list.Element), lru: list.New(), pending: make(map[string]*pendingReplay),
		maxBytes: 16 << 20, maxEntries: 8, ttl: 2 * time.Minute, now: time.Now,
	}
}

func replayCacheKey(path, flightID string, options detection.ReplayOptions) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	// Stored source files are immutable. Include their identity and every replay
	// option so flight-range, sample-interval and phase edits miss immediately.
	encoded, err := json.Marshal(struct {
		Path, FlightID, EngineVersion, PhaseVersion string
		Size, Modified                              int64
		Options                                     detection.ReplayOptions
	}{path, flightID, detection.EngineVersion, detection.PhaseEngineVersion,
		info.Size(), info.ModTime().UnixNano(), options})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func (cache *replayCache) remove(element *list.Element) {
	entry := element.Value.(cachedReplay)
	delete(cache.entries, entry.key)
	cache.bytes -= len(entry.body)
	cache.lru.Remove(element)
}

func (cache *replayCache) get(ctx context.Context, key string, build func() ([]byte, error)) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache.mu.Lock()
	if element := cache.entries[key]; element != nil {
		entry := element.Value.(cachedReplay)
		if cache.now().Before(entry.expires) {
			cache.lru.MoveToFront(element)
			cache.mu.Unlock()
			return entry.body, nil
		}
		cache.remove(element)
	}
	if pending := cache.pending[key]; pending != nil {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-pending.done:
			return pending.body, pending.err
		}
	}
	pending := &pendingReplay{done: make(chan struct{}), err: errors.New("replay build interrupted")}
	cache.pending[key] = pending
	cache.mu.Unlock()
	// Also release waiters if Gin recovers a panic in the builder.
	defer func() {
		cache.mu.Lock()
		delete(cache.pending, key)
		close(pending.done)
		cache.mu.Unlock()
	}()
	body, err := build()
	cache.mu.Lock()
	pending.body, pending.err = body, err
	if err == nil && len(body) <= cache.maxBytes && cache.maxEntries > 0 {
		for cache.lru.Len() > 0 && (cache.bytes+len(body) > cache.maxBytes || cache.lru.Len() >= cache.maxEntries) {
			cache.remove(cache.lru.Back())
		}
		cache.entries[key] = cache.lru.PushFront(cachedReplay{key: key, body: body, expires: cache.now().Add(cache.ttl)})
		cache.bytes += len(body)
	}
	cache.mu.Unlock()
	return body, err
}

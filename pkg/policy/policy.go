// Package policy provides TTL and eviction policy interfaces and
// implementations for fine-grained control over cache entry lifetime
// and eviction decisions.
package policy

import (
	"sync"
	"sync/atomic"
	"time"
)

// --- TTL Policies ---

// TTLPolicy determines the time-to-live for a cache key.
type TTLPolicy interface {
	// GetTTL returns the TTL that should be applied to the given key.
	GetTTL(key string) time.Duration
}

// FixedTTL returns the same TTL for every key.
type FixedTTL struct {
	Duration time.Duration
}

// NewFixedTTL creates a fixed TTL policy.
func NewFixedTTL(d time.Duration) *FixedTTL {
	return &FixedTTL{Duration: d}
}

// GetTTL returns the fixed duration regardless of key.
func (f *FixedTTL) GetTTL(_ string) time.Duration {
	return f.Duration
}

// SlidingTTL extends the TTL each time the key is accessed. The
// initial TTL is set on the first access and refreshed on subsequent
// accesses. Callers should use Touch to record accesses and then
// call GetTTL to get the remaining time.
type SlidingTTL struct {
	BaseTTL time.Duration
	// lastAccess tracks the last access time per key.
	lastAccess sync.Map // key -> time.Time
}

// NewSlidingTTL creates a sliding TTL policy.
func NewSlidingTTL(baseTTL time.Duration) *SlidingTTL {
	return &SlidingTTL{BaseTTL: baseTTL}
}

// Touch records an access for the given key, resetting its TTL
// window.
func (s *SlidingTTL) Touch(key string) {
	s.lastAccess.Store(key, time.Now())
}

// GetTTL returns the base TTL. When used with Touch, the effective
// expiration is BaseTTL after the last Touch call.
func (s *SlidingTTL) GetTTL(_ string) time.Duration {
	return s.BaseTTL
}

// ShouldExpire returns true if the key has not been touched within
// BaseTTL.
func (s *SlidingTTL) ShouldExpire(key string) bool {
	val, ok := s.lastAccess.Load(key)
	if !ok {
		return true
	}
	last := val.(time.Time)
	return time.Since(last) > s.BaseTTL
}

// Remove stops tracking the given key.
func (s *SlidingTTL) Remove(key string) {
	s.lastAccess.Delete(key)
}

// AdaptiveTTL adjusts the TTL based on access frequency. Frequently
// accessed keys get a longer TTL (up to MaxTTL), while infrequently
// accessed keys get a shorter TTL (down to MinTTL).
type AdaptiveTTL struct {
	MinTTL time.Duration
	MaxTTL time.Duration
	// accessCounts tracks how many times each key has been accessed.
	accessCounts sync.Map // key -> *int64
}

// NewAdaptiveTTL creates an adaptive TTL policy.
func NewAdaptiveTTL(minTTL, maxTTL time.Duration) *AdaptiveTTL {
	if minTTL > maxTTL {
		minTTL, maxTTL = maxTTL, minTTL
	}
	return &AdaptiveTTL{
		MinTTL: minTTL,
		MaxTTL: maxTTL,
	}
}

// RecordAccess increments the access counter for a key.
func (a *AdaptiveTTL) RecordAccess(key string) {
	val, _ := a.accessCounts.LoadOrStore(key, new(int64))
	atomic.AddInt64(val.(*int64), 1)
}

// GetTTL returns a TTL proportional to the access count. Keys with
// zero accesses get MinTTL. The TTL increases logarithmically up to
// MaxTTL.
func (a *AdaptiveTTL) GetTTL(key string) time.Duration {
	val, ok := a.accessCounts.Load(key)
	if !ok {
		return a.MinTTL
	}
	count := atomic.LoadInt64(val.(*int64))
	if count <= 0 {
		return a.MinTTL
	}

	// Scale between MinTTL and MaxTTL based on access count.
	// Use a simple linear scaling capped at 100 accesses.
	ratio := float64(count) / 100.0
	if ratio > 1.0 {
		ratio = 1.0
	}

	diff := a.MaxTTL - a.MinTTL
	return a.MinTTL + time.Duration(float64(diff)*ratio)
}

// Reset clears the access count for a key.
func (a *AdaptiveTTL) Reset(key string) {
	a.accessCounts.Delete(key)
}

// AccessCount returns the current access count for a key.
func (a *AdaptiveTTL) AccessCount(key string) int64 {
	val, ok := a.accessCounts.Load(key)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(val.(*int64))
}

// --- Eviction Policies ---

// EvictionDecider decides whether a specific key should be evicted.
type EvictionDecider interface {
	// ShouldEvict returns true if the entry identified by key should
	// be evicted. The stats parameter provides current cache stats
	// for the decision.
	ShouldEvict(key string, stats EvictionStats) bool
}

// EvictionStats provides information used by eviction deciders.
type EvictionStats struct {
	// TotalEntries is the current number of entries in the cache.
	TotalEntries int
	// MaxEntries is the configured maximum.
	MaxEntries int
	// MemoryUsed is the approximate memory used in bytes.
	MemoryUsed int64
	// MaxMemory is the configured memory limit in bytes.
	MaxMemory int64
	// EntryAge is how long ago the entry was created.
	EntryAge time.Duration
	// EntryAccessCount is how many times the entry has been accessed.
	EntryAccessCount int64
	// EntryLastAccess is the time of the last access.
	EntryLastAccess time.Time
}

// CapacityEviction evicts entries when the cache exceeds its
// capacity threshold.
type CapacityEviction struct {
	// Threshold is a fraction (0.0-1.0) of max capacity at which
	// eviction begins.
	Threshold float64
}

// NewCapacityEviction creates a capacity-based eviction policy.
// A threshold of 0.9 means eviction starts at 90% capacity.
func NewCapacityEviction(threshold float64) *CapacityEviction {
	if threshold <= 0 || threshold > 1.0 {
		threshold = 0.9
	}
	return &CapacityEviction{Threshold: threshold}
}

// ShouldEvict returns true if the cache is above the capacity
// threshold.
func (c *CapacityEviction) ShouldEvict(_ string, stats EvictionStats) bool {
	if stats.MaxEntries <= 0 {
		return false
	}
	ratio := float64(stats.TotalEntries) / float64(stats.MaxEntries)
	return ratio >= c.Threshold
}

// AgeEviction evicts entries older than a maximum age.
type AgeEviction struct {
	MaxAge time.Duration
}

// NewAgeEviction creates an age-based eviction policy.
func NewAgeEviction(maxAge time.Duration) *AgeEviction {
	return &AgeEviction{MaxAge: maxAge}
}

// ShouldEvict returns true if the entry is older than MaxAge.
func (a *AgeEviction) ShouldEvict(_ string, stats EvictionStats) bool {
	return stats.EntryAge > a.MaxAge
}

// FrequencyEviction evicts entries accessed fewer than MinAccesses
// times.
type FrequencyEviction struct {
	MinAccesses int64
}

// NewFrequencyEviction creates a frequency-based eviction policy.
func NewFrequencyEviction(minAccesses int64) *FrequencyEviction {
	return &FrequencyEviction{MinAccesses: minAccesses}
}

// ShouldEvict returns true if the entry has been accessed fewer
// than MinAccesses times.
func (f *FrequencyEviction) ShouldEvict(_ string, stats EvictionStats) bool {
	return stats.EntryAccessCount < f.MinAccesses
}

// CompositeEviction combines multiple eviction policies. An entry
// is evicted if ANY of the sub-policies says it should be evicted.
type CompositeEviction struct {
	policies []EvictionDecider
}

// NewCompositeEviction creates a composite eviction policy.
func NewCompositeEviction(policies ...EvictionDecider) *CompositeEviction {
	return &CompositeEviction{policies: policies}
}

// ShouldEvict returns true if any sub-policy says the entry should
// be evicted.
func (c *CompositeEviction) ShouldEvict(key string, stats EvictionStats) bool {
	for _, p := range c.policies {
		if p.ShouldEvict(key, stats) {
			return true
		}
	}
	return false
}

// Compile-time checks
var (
	_ TTLPolicy       = (*FixedTTL)(nil)
	_ TTLPolicy       = (*SlidingTTL)(nil)
	_ TTLPolicy       = (*AdaptiveTTL)(nil)
	_ EvictionDecider = (*CapacityEviction)(nil)
	_ EvictionDecider = (*AgeEviction)(nil)
	_ EvictionDecider = (*FrequencyEviction)(nil)
	_ EvictionDecider = (*CompositeEviction)(nil)
)

package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// --- FixedTTL tests ---

func TestFixedTTL_GetTTL(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		key      string
	}{
		{name: "5 minutes", duration: 5 * time.Minute, key: "any"},
		{name: "zero", duration: 0, key: "key"},
		{name: "1 hour", duration: time.Hour, key: "another"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFixedTTL(tt.duration)
			assert.Equal(t, tt.duration, f.GetTTL(tt.key))
		})
	}
}

// --- SlidingTTL tests ---

func TestSlidingTTL_GetTTL(t *testing.T) {
	s := NewSlidingTTL(10 * time.Minute)
	assert.Equal(t, 10*time.Minute, s.GetTTL("any"))
}

func TestSlidingTTL_Touch(t *testing.T) {
	s := NewSlidingTTL(50 * time.Millisecond)

	// Not touched -> should expire
	assert.True(t, s.ShouldExpire("key"))

	// Touch -> should not expire immediately
	s.Touch("key")
	assert.False(t, s.ShouldExpire("key"))

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)
	assert.True(t, s.ShouldExpire("key"))

	// Touch again -> resets
	s.Touch("key")
	assert.False(t, s.ShouldExpire("key"))
}

func TestSlidingTTL_Remove(t *testing.T) {
	s := NewSlidingTTL(time.Minute)
	s.Touch("key")
	assert.False(t, s.ShouldExpire("key"))

	s.Remove("key")
	assert.True(t, s.ShouldExpire("key"))
}

// --- AdaptiveTTL tests ---

func TestAdaptiveTTL_GetTTL(t *testing.T) {
	tests := []struct {
		name      string
		minTTL    time.Duration
		maxTTL    time.Duration
		accesses  int
		expectMin time.Duration
		expectMax time.Duration
	}{
		{
			name:      "no accesses returns minTTL",
			minTTL:    time.Minute,
			maxTTL:    time.Hour,
			accesses:  0,
			expectMin: time.Minute,
			expectMax: time.Minute,
		},
		{
			name:      "many accesses returns maxTTL",
			minTTL:    time.Minute,
			maxTTL:    time.Hour,
			accesses:  200,
			expectMin: time.Hour,
			expectMax: time.Hour,
		},
		{
			name:      "50 accesses returns ~midpoint",
			minTTL:    time.Minute,
			maxTTL:    time.Hour,
			accesses:  50,
			expectMin: 25 * time.Minute,
			expectMax: 35 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewAdaptiveTTL(tt.minTTL, tt.maxTTL)
			for i := 0; i < tt.accesses; i++ {
				a.RecordAccess("key")
			}
			ttl := a.GetTTL("key")
			assert.GreaterOrEqual(t, ttl, tt.expectMin)
			assert.LessOrEqual(t, ttl, tt.expectMax)
		})
	}
}

func TestAdaptiveTTL_SwappedMinMax(t *testing.T) {
	a := NewAdaptiveTTL(time.Hour, time.Minute) // swapped
	assert.Equal(t, time.Minute, a.MinTTL)
	assert.Equal(t, time.Hour, a.MaxTTL)
}

func TestAdaptiveTTL_AccessCount(t *testing.T) {
	a := NewAdaptiveTTL(time.Minute, time.Hour)
	assert.Equal(t, int64(0), a.AccessCount("key"))

	a.RecordAccess("key")
	a.RecordAccess("key")
	assert.Equal(t, int64(2), a.AccessCount("key"))
}

func TestAdaptiveTTL_Reset(t *testing.T) {
	a := NewAdaptiveTTL(time.Minute, time.Hour)
	a.RecordAccess("key")
	a.RecordAccess("key")
	a.Reset("key")
	assert.Equal(t, int64(0), a.AccessCount("key"))
	assert.Equal(t, time.Minute, a.GetTTL("key"))
}

func TestAdaptiveTTL_GetTTL_ZeroCount(t *testing.T) {
	// Test the case where accessCounts has an entry but the count is 0 or negative
	a := NewAdaptiveTTL(time.Minute, time.Hour)
	// RecordAccess then manually set count to 0 to test branch
	a.RecordAccess("key")
	// Get the stored pointer and set it to 0
	val, _ := a.accessCounts.Load("key")
	ptr := val.(*int64)
	*ptr = 0

	// Now GetTTL should return MinTTL because count <= 0
	ttl := a.GetTTL("key")
	assert.Equal(t, time.Minute, ttl)
}

// --- CapacityEviction tests ---

func TestCapacityEviction_ShouldEvict(t *testing.T) {
	tests := []struct {
		name      string
		threshold float64
		total     int
		max       int
		expected  bool
	}{
		{name: "below threshold", threshold: 0.9, total: 80, max: 100, expected: false},
		{name: "at threshold", threshold: 0.9, total: 90, max: 100, expected: true},
		{name: "above threshold", threshold: 0.9, total: 95, max: 100, expected: true},
		{name: "zero max", threshold: 0.9, total: 5, max: 0, expected: false},
		{name: "full", threshold: 1.0, total: 100, max: 100, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := NewCapacityEviction(tt.threshold)
			stats := EvictionStats{
				TotalEntries: tt.total,
				MaxEntries:   tt.max,
			}
			assert.Equal(t, tt.expected, ce.ShouldEvict("key", stats))
		})
	}
}

func TestCapacityEviction_InvalidThreshold(t *testing.T) {
	ce := NewCapacityEviction(0)
	assert.Equal(t, 0.9, ce.Threshold)

	ce = NewCapacityEviction(1.5)
	assert.Equal(t, 0.9, ce.Threshold)
}

// --- AgeEviction tests ---

func TestAgeEviction_ShouldEvict(t *testing.T) {
	tests := []struct {
		name     string
		maxAge   time.Duration
		age      time.Duration
		expected bool
	}{
		{name: "young entry", maxAge: time.Hour, age: 5 * time.Minute, expected: false},
		{name: "old entry", maxAge: time.Hour, age: 2 * time.Hour, expected: true},
		{name: "at boundary", maxAge: time.Hour, age: time.Hour, expected: false},
		{name: "just over", maxAge: time.Hour, age: time.Hour + time.Millisecond, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ae := NewAgeEviction(tt.maxAge)
			stats := EvictionStats{EntryAge: tt.age}
			assert.Equal(t, tt.expected, ae.ShouldEvict("key", stats))
		})
	}
}

// --- FrequencyEviction tests ---

func TestFrequencyEviction_ShouldEvict(t *testing.T) {
	tests := []struct {
		name        string
		minAccesses int64
		actual      int64
		expected    bool
	}{
		{name: "enough accesses", minAccesses: 5, actual: 10, expected: false},
		{name: "not enough", minAccesses: 5, actual: 3, expected: true},
		{name: "exact minimum", minAccesses: 5, actual: 5, expected: false},
		{name: "zero accesses", minAccesses: 1, actual: 0, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fe := NewFrequencyEviction(tt.minAccesses)
			stats := EvictionStats{EntryAccessCount: tt.actual}
			assert.Equal(t, tt.expected, fe.ShouldEvict("key", stats))
		})
	}
}

// --- CompositeEviction tests ---

func TestCompositeEviction_ShouldEvict(t *testing.T) {
	tests := []struct {
		name     string
		policies []EvictionDecider
		stats    EvictionStats
		expected bool
	}{
		{
			name:     "no policies -> no eviction",
			expected: false,
		},
		{
			name: "one says yes -> evict",
			policies: []EvictionDecider{
				NewAgeEviction(time.Hour),
			},
			stats:    EvictionStats{EntryAge: 2 * time.Hour},
			expected: true,
		},
		{
			name: "all say no -> no eviction",
			policies: []EvictionDecider{
				NewAgeEviction(time.Hour),
				NewFrequencyEviction(5),
			},
			stats: EvictionStats{
				EntryAge:         30 * time.Minute,
				EntryAccessCount: 10,
			},
			expected: false,
		},
		{
			name: "one of many says yes -> evict",
			policies: []EvictionDecider{
				NewAgeEviction(time.Hour), // no
				NewFrequencyEviction(5),   // yes (count=0)
				NewCapacityEviction(0.9),  // no
			},
			stats: EvictionStats{
				EntryAge:         30 * time.Minute,
				EntryAccessCount: 0,
				TotalEntries:     50,
				MaxEntries:       100,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ce := NewCompositeEviction(tt.policies...)
			assert.Equal(t, tt.expected, ce.ShouldEvict("key", tt.stats))
		})
	}
}

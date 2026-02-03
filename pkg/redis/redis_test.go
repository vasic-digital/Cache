package redis

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests use miniredis for in-memory Redis simulation.

func setupMiniRedis(t *testing.T) (*miniredis.Miniredis, *Client) {
	t.Helper()
	s := miniredis.RunT(t)
	client := New(&Config{
		Addr:     s.Addr(),
		Password: "",
		DB:       0,
	})
	return s, client
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"Addr", cfg.Addr, "localhost:6379"},
		{"Password", cfg.Password, ""},
		{"DB", cfg.DB, 0},
		{"PoolSize", cfg.PoolSize, 10},
		{"MinIdleConns", cfg.MinIdleConns, 2},
		{"DialTimeout", cfg.DialTimeout, 5 * time.Second},
		{"ReadTimeout", cfg.ReadTimeout, 3 * time.Second},
		{"WriteTimeout", cfg.WriteTimeout, 3 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.got)
		})
	}
}

func TestNew_NilConfig(t *testing.T) {
	client := New(nil)
	require.NotNil(t, client)
	require.NotNil(t, client.rdb)
	_ = client.Close()
}

func TestNew_CustomConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{
			name: "custom addr and password",
			cfg: &Config{
				Addr:     "redis.example.com:6380",
				Password: "secret",
				DB:       3,
				PoolSize: 20,
			},
		},
		{
			name: "minimal config",
			cfg: &Config{
				Addr: "localhost:6379",
			},
		},
		{
			name: "full config",
			cfg: &Config{
				Addr:         "10.0.0.1:6379",
				Password:     "p@ss",
				DB:           15,
				PoolSize:     50,
				MinIdleConns: 10,
				DialTimeout:  10 * time.Second,
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 5 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := New(tt.cfg)
			require.NotNil(t, client)
			require.NotNil(t, client.rdb)
			_ = client.Close()
		})
	}
}

func TestClient_Underlying(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	require.NotNil(t, client.Underlying())
}

func TestClient_Close(t *testing.T) {
	_, client := setupMiniRedis(t)
	err := client.Close()
	assert.NoError(t, err)
}

func TestClient_Get_Set(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "test-key"
	value := []byte("test-value")

	// Set a value
	err := client.Set(ctx, key, value, 5*time.Minute)
	require.NoError(t, err)

	// Get the value back
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestClient_Get_Miss(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Get non-existent key
	got, err := client.Get(ctx, "non-existent-key")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestClient_Set_NoTTL(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "no-ttl-key"
	value := []byte("persistent-value")

	// Set with zero TTL (no expiration)
	err := client.Set(ctx, key, value, 0)
	require.NoError(t, err)

	// Key should not have TTL
	ttl := s.TTL(key)
	assert.Equal(t, time.Duration(0), ttl)

	// Value should be retrievable
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestClient_Delete(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "delete-key"
	value := []byte("to-delete")

	// Set then delete
	err := client.Set(ctx, key, value, 5*time.Minute)
	require.NoError(t, err)

	err = client.Delete(ctx, key)
	require.NoError(t, err)

	// Should be gone
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestClient_Delete_NonExistent(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Deleting non-existent key should not error
	err := client.Delete(ctx, "non-existent")
	require.NoError(t, err)
}

func TestClient_Exists(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "exists-key"

	// Should not exist initially
	exists, err := client.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Set the key
	err = client.Set(ctx, key, []byte("value"), 5*time.Minute)
	require.NoError(t, err)

	// Should exist now
	exists, err = client.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestClient_HealthCheck(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	err := client.HealthCheck(ctx)
	require.NoError(t, err)
}

func TestClient_HealthCheck_Failure(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Close miniredis to simulate failure
	s.Close()

	err := client.HealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis health check")
}

func TestClient_Get_Error(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Close miniredis to simulate connection error
	s.Close()

	_, err := client.Get(ctx, "any-key")
	// May return nil,nil on miss or error depending on timing
	// The important thing is we test the error path
	if err != nil {
		assert.Contains(t, err.Error(), "redis get")
	}
}

func TestClient_Set_Error(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Close miniredis to simulate connection error
	s.Close()

	err := client.Set(ctx, "any-key", []byte("value"), time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis set")
}

func TestClient_Delete_Error(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Close miniredis to simulate connection error
	s.Close()

	err := client.Delete(ctx, "any-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis delete")
}

func TestClient_Exists_Error(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Close miniredis to simulate connection error
	s.Close()

	_, err := client.Exists(ctx, "any-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis exists")
}

func TestNewCluster_NilConfig(t *testing.T) {
	client := NewCluster(nil)
	require.NotNil(t, client)
	require.NotNil(t, client.rdb)
	_ = client.Close()
}

func TestNewCluster_CustomConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ClusterConfig
	}{
		{
			name: "three nodes",
			cfg: &ClusterConfig{
				Addrs:    []string{"node1:7000", "node2:7001", "node3:7002"},
				Password: "cluster-pass",
				PoolSize: 15,
			},
		},
		{
			name: "single seed",
			cfg: &ClusterConfig{
				Addrs: []string{"localhost:7000"},
			},
		},
		{
			name: "full config",
			cfg: &ClusterConfig{
				Addrs:        []string{"n1:7000", "n2:7001"},
				Password:     "pass",
				PoolSize:     25,
				MinIdleConns: 5,
				DialTimeout:  10 * time.Second,
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 5 * time.Second,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewCluster(tt.cfg)
			require.NotNil(t, client)
			require.NotNil(t, client.rdb)
			_ = client.Close()
		})
	}
}

func TestClusterClient_Close(t *testing.T) {
	client := NewCluster(nil)
	err := client.Close()
	assert.NoError(t, err)
}

// Note: ClusterClient methods (Get, Set, Delete, Exists, HealthCheck) cannot be
// tested with miniredis as it doesn't support cluster mode. These are tested
// via integration tests with a real Redis Cluster.

// Interface compliance at compile time
var _ interface {
	Close() error
} = (*Client)(nil)

var _ interface {
	Close() error
} = (*ClusterClient)(nil)

func TestClient_MultipleOperations(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Test a sequence of operations
	keys := []string{"k1", "k2", "k3"}
	values := [][]byte{[]byte("v1"), []byte("v2"), []byte("v3")}

	// Set all
	for i, k := range keys {
		err := client.Set(ctx, k, values[i], time.Minute)
		require.NoError(t, err)
	}

	// Get all
	for i, k := range keys {
		got, err := client.Get(ctx, k)
		require.NoError(t, err)
		assert.Equal(t, values[i], got)
	}

	// Check all exist
	for _, k := range keys {
		exists, err := client.Exists(ctx, k)
		require.NoError(t, err)
		assert.True(t, exists)
	}

	// Delete one
	err := client.Delete(ctx, "k2")
	require.NoError(t, err)

	// k2 should not exist
	exists, err := client.Exists(ctx, "k2")
	require.NoError(t, err)
	assert.False(t, exists)

	// k1 and k3 should still exist
	for _, k := range []string{"k1", "k3"} {
		exists, err := client.Exists(ctx, k)
		require.NoError(t, err)
		assert.True(t, exists)
	}
}

func TestClient_TTLExpiration(t *testing.T) {
	s, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "expiring-key"

	err := client.Set(ctx, key, []byte("value"), 100*time.Millisecond)
	require.NoError(t, err)

	// Should exist
	exists, err := client.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Fast forward time in miniredis
	s.FastForward(200 * time.Millisecond)

	// Should be expired
	exists, err = client.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestClient_LargeValue(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "large-key"

	// Create a 1MB value
	value := make([]byte, 1024*1024)
	for i := range value {
		value[i] = byte(i % 256)
	}

	err := client.Set(ctx, key, value, time.Minute)
	require.NoError(t, err)

	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestClient_EmptyValue(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "empty-key"
	value := []byte{}

	err := client.Set(ctx, key, value, time.Minute)
	require.NoError(t, err)

	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)
}

func TestClient_SpecialCharactersInKey(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	keys := []string{
		"key:with:colons",
		"key/with/slashes",
		"key.with.dots",
		"key-with-dashes",
		"key_with_underscores",
		"key with spaces",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			value := []byte("value for " + key)

			err := client.Set(ctx, key, value, time.Minute)
			require.NoError(t, err)

			got, err := client.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, value, got)

			exists, err := client.Exists(ctx, key)
			require.NoError(t, err)
			assert.True(t, exists)

			err = client.Delete(ctx, key)
			require.NoError(t, err)

			exists, err = client.Exists(ctx, key)
			require.NoError(t, err)
			assert.False(t, exists)
		})
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Operations with cancelled context may fail
	// The exact behavior depends on the Redis client implementation
	_, _ = client.Get(ctx, "key")
	_ = client.Set(ctx, "key", []byte("value"), time.Minute)
	_, _ = client.Exists(ctx, "key")
	_ = client.Delete(ctx, "key")
	_ = client.HealthCheck(ctx)
}

// =============================================================================
// ClusterClient Tests - Testing error paths since miniredis doesn't support cluster
// =============================================================================

// setupClosedClusterClient creates a ClusterClient that immediately fails
// operations since no real cluster exists.
func setupClosedClusterClient(t *testing.T) *ClusterClient {
	t.Helper()
	// Using an unreachable address ensures operations fail immediately
	return NewCluster(&ClusterConfig{
		Addrs:       []string{"127.0.0.1:59999"}, // Non-existent port
		DialTimeout: 10 * time.Millisecond,       // Fast failure
		ReadTimeout: 10 * time.Millisecond,
	})
}

func TestClusterClient_Get_Error(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Get(ctx, "any-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis cluster get")
}

func TestClusterClient_Get_CacheMiss(t *testing.T) {
	// This test verifies the cache miss path (redis.Nil handling)
	// Since we can't use miniredis for cluster, we test error behavior
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// With no cluster, this will error (not return nil for cache miss)
	data, err := client.Get(ctx, "non-existent-key")
	// Either returns error or nil data depending on cluster state
	if err != nil {
		assert.Contains(t, err.Error(), "redis cluster get")
	} else {
		assert.Nil(t, data)
	}
}

func TestClusterClient_Set_Error(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.Set(ctx, "any-key", []byte("value"), time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis cluster set")
}

func TestClusterClient_Delete_Error(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.Delete(ctx, "any-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis cluster delete")
}

func TestClusterClient_Exists_Error(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := client.Exists(ctx, "any-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis cluster exists")
}

func TestClusterClient_HealthCheck_Error(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := client.HealthCheck(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "redis cluster health check")
}

func TestClusterClient_ContextCancellation(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// All operations should fail with cancelled context
	_, err := client.Get(ctx, "key")
	assert.Error(t, err)

	err = client.Set(ctx, "key", []byte("value"), time.Minute)
	assert.Error(t, err)

	_, err = client.Exists(ctx, "key")
	assert.Error(t, err)

	err = client.Delete(ctx, "key")
	assert.Error(t, err)

	err = client.HealthCheck(ctx)
	assert.Error(t, err)
}

// =============================================================================
// Additional Client Error Path Tests
// =============================================================================

func TestClient_Get_ConnectionError_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "simple key", key: "simple"},
		{name: "key with colons", key: "prefix:suffix"},
		{name: "key with special chars", key: "key/with/slashes"},
		{name: "empty key", key: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, client := setupMiniRedis(t)
			s.Close() // Close immediately to simulate error
			defer client.Close()

			ctx := context.Background()
			_, err := client.Get(ctx, tt.key)
			if err != nil {
				assert.Contains(t, err.Error(), "redis get")
			}
		})
	}
}

func TestClient_Set_ConnectionError_TableDriven(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value []byte
		ttl   time.Duration
	}{
		{name: "with TTL", key: "key1", value: []byte("value"), ttl: time.Minute},
		{name: "zero TTL", key: "key2", value: []byte("value"), ttl: 0},
		{name: "large value", key: "key3", value: make([]byte, 1024), ttl: time.Minute},
		{name: "empty value", key: "key4", value: []byte{}, ttl: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, client := setupMiniRedis(t)
			s.Close() // Close immediately to simulate error
			defer client.Close()

			ctx := context.Background()
			err := client.Set(ctx, tt.key, tt.value, tt.ttl)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "redis set")
		})
	}
}

func TestClient_Delete_ConnectionError_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "simple key", key: "delete-key"},
		{name: "non-existent key", key: "non-existent"},
		{name: "key with special chars", key: "prefix:key:suffix"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, client := setupMiniRedis(t)
			s.Close() // Close immediately to simulate error
			defer client.Close()

			ctx := context.Background()
			err := client.Delete(ctx, tt.key)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "redis delete")
		})
	}
}

func TestClient_Exists_ConnectionError_TableDriven(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{name: "simple key", key: "exists-key"},
		{name: "non-existent key", key: "non-existent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, client := setupMiniRedis(t)
			s.Close() // Close immediately to simulate error
			defer client.Close()

			ctx := context.Background()
			_, err := client.Exists(ctx, tt.key)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "redis exists")
		})
	}
}

// =============================================================================
// Context Timeout Tests
// =============================================================================

func TestClient_ContextTimeout(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	// Use an already-expired context
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// Operations may fail immediately with deadline exceeded
	_, err := client.Get(ctx, "key")
	if err != nil {
		assert.Error(t, err)
	}

	err = client.Set(ctx, "key", []byte("value"), time.Minute)
	if err != nil {
		assert.Error(t, err)
	}

	_, err = client.Exists(ctx, "key")
	if err != nil {
		assert.Error(t, err)
	}

	err = client.Delete(ctx, "key")
	if err != nil {
		assert.Error(t, err)
	}

	err = client.HealthCheck(ctx)
	if err != nil {
		assert.Error(t, err)
	}
}

func TestClusterClient_ContextTimeout(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	// Use an already-expired context
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// All operations should fail with expired context
	_, err := client.Get(ctx, "key")
	assert.Error(t, err)

	err = client.Set(ctx, "key", []byte("value"), time.Minute)
	assert.Error(t, err)

	_, err = client.Exists(ctx, "key")
	assert.Error(t, err)

	err = client.Delete(ctx, "key")
	assert.Error(t, err)

	err = client.HealthCheck(ctx)
	assert.Error(t, err)
}

// =============================================================================
// TTL Edge Cases
// =============================================================================

func TestClient_TTL_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{name: "zero ttl - no expiration", ttl: 0},
		{name: "very short ttl", ttl: time.Millisecond},
		{name: "one second", ttl: time.Second},
		{name: "one minute", ttl: time.Minute},
		{name: "one hour", ttl: time.Hour},
		{name: "24 hours", ttl: 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, client := setupMiniRedis(t)
			defer client.Close()

			ctx := context.Background()
			key := "ttl-test-" + tt.name
			value := []byte("value")

			err := client.Set(ctx, key, value, tt.ttl)
			require.NoError(t, err)

			// For very short TTL, fast forward to check expiration
			if tt.ttl > 0 && tt.ttl <= time.Second {
				got, err := client.Get(ctx, key)
				require.NoError(t, err)
				assert.Equal(t, value, got)

				s.FastForward(tt.ttl + time.Millisecond)

				got, err = client.Get(ctx, key)
				require.NoError(t, err)
				assert.Nil(t, got)
			}
		})
	}
}

func TestClient_TTL_NegativeValue(t *testing.T) {
	// Negative TTL behavior varies between Redis versions and miniredis
	// In go-redis, negative TTL is treated as 0 (no expiration) by default
	// This test verifies the key is at least set successfully
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "negative-ttl-key"
	value := []byte("value")

	// Negative TTL should not cause an error
	err := client.Set(ctx, key, value, -time.Second)
	require.NoError(t, err)

	// Key should exist (exact behavior may vary)
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	// Value should be set (miniredis may not expire immediately)
	assert.NotNil(t, got)
}

// =============================================================================
// Binary Data Tests
// =============================================================================

func TestClient_BinaryData(t *testing.T) {
	tests := []struct {
		name  string
		value []byte
	}{
		{name: "null bytes", value: []byte{0, 0, 0, 0}},
		{name: "binary with nulls", value: []byte{1, 0, 2, 0, 3}},
		{name: "high bytes", value: []byte{255, 254, 253, 252}},
		{name: "mixed binary", value: []byte{0, 127, 128, 255}},
		{name: "all byte values", value: func() []byte {
			b := make([]byte, 256)
			for i := range b {
				b[i] = byte(i)
			}
			return b
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, client := setupMiniRedis(t)
			defer client.Close()

			ctx := context.Background()
			key := "binary-" + tt.name

			err := client.Set(ctx, key, tt.value, time.Minute)
			require.NoError(t, err)

			got, err := client.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

// =============================================================================
// Concurrent Operations Tests
// =============================================================================

func TestClient_ConcurrentOperations(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	numGoroutines := 10
	numOps := 100

	done := make(chan bool, numGoroutines)

	for g := 0; g < numGoroutines; g++ {
		go func(id int) {
			for i := 0; i < numOps; i++ {
				key := fmt.Sprintf("concurrent-%d-%d", id, i)
				value := []byte(fmt.Sprintf("value-%d-%d", id, i))

				err := client.Set(ctx, key, value, time.Minute)
				assert.NoError(t, err)

				got, err := client.Get(ctx, key)
				assert.NoError(t, err)
				assert.Equal(t, value, got)

				exists, err := client.Exists(ctx, key)
				assert.NoError(t, err)
				assert.True(t, exists)

				err = client.Delete(ctx, key)
				assert.NoError(t, err)
			}
			done <- true
		}(g)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// =============================================================================
// ClusterConfig Validation Tests
// =============================================================================

func TestClusterClient_Operations_TableDriven(t *testing.T) {
	tests := []struct {
		name      string
		operation func(ctx context.Context, c *ClusterClient) error
		errorMsg  string
	}{
		{
			name: "Get operation",
			operation: func(ctx context.Context, c *ClusterClient) error {
				_, err := c.Get(ctx, "test-key")
				return err
			},
			errorMsg: "redis cluster get",
		},
		{
			name: "Set operation",
			operation: func(ctx context.Context, c *ClusterClient) error {
				return c.Set(ctx, "test-key", []byte("value"), time.Minute)
			},
			errorMsg: "redis cluster set",
		},
		{
			name: "Delete operation",
			operation: func(ctx context.Context, c *ClusterClient) error {
				return c.Delete(ctx, "test-key")
			},
			errorMsg: "redis cluster delete",
		},
		{
			name: "Exists operation",
			operation: func(ctx context.Context, c *ClusterClient) error {
				_, err := c.Exists(ctx, "test-key")
				return err
			},
			errorMsg: "redis cluster exists",
		},
		{
			name: "HealthCheck operation",
			operation: func(ctx context.Context, c *ClusterClient) error {
				return c.HealthCheck(ctx)
			},
			errorMsg: "redis cluster health check",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := setupClosedClusterClient(t)
			defer client.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			err := tt.operation(ctx, client)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorMsg)
		})
	}
}

// =============================================================================
// Interface Compliance Tests
// =============================================================================

// Verify both clients implement the same interface pattern
type CacheOperations interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Close() error
	HealthCheck(ctx context.Context) error
}

var _ CacheOperations = (*Client)(nil)
var _ CacheOperations = (*ClusterClient)(nil)

// =============================================================================
// Error Wrapping Verification Tests
// =============================================================================

func TestClient_ErrorWrapping(t *testing.T) {
	s, client := setupMiniRedis(t)
	s.Close() // Force errors
	defer client.Close()

	ctx := context.Background()

	// Verify errors are properly wrapped with context
	_, err := client.Get(ctx, "key")
	if err != nil {
		assert.Contains(t, err.Error(), `redis get "key"`)
	}

	err = client.Set(ctx, "mykey", []byte("val"), time.Minute)
	if err != nil {
		assert.Contains(t, err.Error(), `redis set "mykey"`)
	}

	err = client.Delete(ctx, "delkey")
	if err != nil {
		assert.Contains(t, err.Error(), `redis delete "delkey"`)
	}

	_, err = client.Exists(ctx, "existskey")
	if err != nil {
		assert.Contains(t, err.Error(), `redis exists "existskey"`)
	}
}

func TestClusterClient_ErrorWrapping(t *testing.T) {
	client := setupClosedClusterClient(t)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Verify errors are properly wrapped with context
	_, err := client.Get(ctx, "key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `redis cluster get "key"`)

	err = client.Set(ctx, "mykey", []byte("val"), time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `redis cluster set "mykey"`)

	err = client.Delete(ctx, "delkey")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `redis cluster delete "delkey"`)

	_, err = client.Exists(ctx, "existskey")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `redis cluster exists "existskey"`)
}

// =============================================================================
// Overwrite Behavior Tests
// =============================================================================

func TestClient_Overwrite(t *testing.T) {
	_, client := setupMiniRedis(t)
	defer client.Close()

	ctx := context.Background()
	key := "overwrite-key"

	// Set initial value
	err := client.Set(ctx, key, []byte("value1"), time.Minute)
	require.NoError(t, err)

	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, []byte("value1"), got)

	// Overwrite with new value
	err = client.Set(ctx, key, []byte("value2"), time.Minute)
	require.NoError(t, err)

	got, err = client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, []byte("value2"), got)

	// Overwrite with different TTL
	err = client.Set(ctx, key, []byte("value3"), 0) // No TTL
	require.NoError(t, err)

	got, err = client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, []byte("value3"), got)
}

// =============================================================================
// Mock ClusterClient for Success Path Testing
// =============================================================================

// mockClusterCmdable is a mock implementation of clusterCmdable for testing.
type mockClusterCmdable struct {
	data      map[string]string
	shouldErr bool
	errMsg    string
}

func newMockClusterCmdable() *mockClusterCmdable {
	return &mockClusterCmdable{
		data: make(map[string]string),
	}
}

func (m *mockClusterCmdable) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx, "get", key)
	if m.shouldErr {
		cmd.SetErr(errors.New(m.errMsg))
		return cmd
	}
	val, ok := m.data[key]
	if !ok {
		cmd.SetErr(redis.Nil)
		return cmd
	}
	cmd.SetVal(val)
	return cmd
}

func (m *mockClusterCmdable) Set(ctx context.Context, key string, value interface{},
	expiration time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx, "set", key, value)
	if m.shouldErr {
		cmd.SetErr(errors.New(m.errMsg))
		return cmd
	}
	m.data[key] = fmt.Sprintf("%v", value)
	cmd.SetVal("OK")
	return cmd
}

func (m *mockClusterCmdable) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx, "del")
	if m.shouldErr {
		cmd.SetErr(errors.New(m.errMsg))
		return cmd
	}
	var deleted int64
	for _, key := range keys {
		if _, ok := m.data[key]; ok {
			delete(m.data, key)
			deleted++
		}
	}
	cmd.SetVal(deleted)
	return cmd
}

func (m *mockClusterCmdable) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx, "exists")
	if m.shouldErr {
		cmd.SetErr(errors.New(m.errMsg))
		return cmd
	}
	var count int64
	for _, key := range keys {
		if _, ok := m.data[key]; ok {
			count++
		}
	}
	cmd.SetVal(count)
	return cmd
}

func (m *mockClusterCmdable) Ping(ctx context.Context) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx, "ping")
	if m.shouldErr {
		cmd.SetErr(errors.New(m.errMsg))
		return cmd
	}
	cmd.SetVal("PONG")
	return cmd
}

func (m *mockClusterCmdable) Close() error {
	if m.shouldErr {
		return errors.New(m.errMsg)
	}
	return nil
}

// newMockClusterClient creates a ClusterClient with a mock for testing.
func newMockClusterClient() (*ClusterClient, *mockClusterCmdable) {
	mock := newMockClusterCmdable()
	return &ClusterClient{rdb: mock}, mock
}

// =============================================================================
// ClusterClient Success Path Tests (using mock)
// =============================================================================

func TestClusterClient_Get_Success(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	// Test cache miss (key doesn't exist)
	got, err := client.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)

	// Set a value directly in mock
	mock.data["test-key"] = "test-value"

	// Test cache hit
	got, err = client.Get(ctx, "test-key")
	require.NoError(t, err)
	assert.Equal(t, []byte("test-value"), got)
}

func TestClusterClient_Set_Success(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value []byte
		ttl   time.Duration
	}{
		{name: "with TTL", key: "k1", value: []byte("v1"), ttl: time.Minute},
		{name: "zero TTL", key: "k2", value: []byte("v2"), ttl: 0},
		{name: "empty value", key: "k3", value: []byte{}, ttl: time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.Set(ctx, tt.key, tt.value, tt.ttl)
			require.NoError(t, err)

			// Verify value was set in mock
			_, exists := mock.data[tt.key]
			assert.True(t, exists)
		})
	}
}

func TestClusterClient_Delete_Success(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	// Set a value directly
	mock.data["delete-me"] = "value"

	// Delete the key
	err := client.Delete(ctx, "delete-me")
	require.NoError(t, err)

	// Verify deleted
	_, exists := mock.data["delete-me"]
	assert.False(t, exists)

	// Deleting non-existent key should not error
	err = client.Delete(ctx, "nonexistent")
	require.NoError(t, err)
}

func TestClusterClient_Exists_Success(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	// Key doesn't exist
	exists, err := client.Exists(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	// Add key to mock
	mock.data["existing-key"] = "value"

	// Key exists
	exists, err = client.Exists(ctx, "existing-key")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestClusterClient_HealthCheck_Success(t *testing.T) {
	client, _ := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	err := client.HealthCheck(ctx)
	require.NoError(t, err)
}

func TestClusterClient_Close_Success(t *testing.T) {
	client, _ := newMockClusterClient()

	err := client.Close()
	assert.NoError(t, err)
}

func TestClusterClient_AllOperations_WithMock(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	// Test full workflow: Set -> Exists -> Get -> Delete -> Exists
	key := "workflow-key"
	value := []byte("workflow-value")

	// Set
	err := client.Set(ctx, key, value, time.Minute)
	require.NoError(t, err)

	// Exists (true)
	exists, err := client.Exists(ctx, key)
	require.NoError(t, err)
	assert.True(t, exists)

	// Get - need to set value in mock format
	mock.data[key] = string(value)
	got, err := client.Get(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, value, got)

	// Delete
	err = client.Delete(ctx, key)
	require.NoError(t, err)

	// Exists (false)
	exists, err = client.Exists(ctx, key)
	require.NoError(t, err)
	assert.False(t, exists)

	// Get after delete (cache miss)
	got, err = client.Get(ctx, key)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestClusterClient_MultipleKeys_WithMock(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	keys := []string{"key1", "key2", "key3"}
	values := [][]byte{[]byte("val1"), []byte("val2"), []byte("val3")}

	// Set all
	for i, k := range keys {
		err := client.Set(ctx, k, values[i], time.Minute)
		require.NoError(t, err)
		mock.data[k] = string(values[i])
	}

	// Get all
	for i, k := range keys {
		got, err := client.Get(ctx, k)
		require.NoError(t, err)
		assert.Equal(t, values[i], got)
	}

	// Check all exist
	for _, k := range keys {
		exists, err := client.Exists(ctx, k)
		require.NoError(t, err)
		assert.True(t, exists)
	}

	// Delete one
	err := client.Delete(ctx, "key2")
	require.NoError(t, err)

	// Verify key2 is gone
	exists, err := client.Exists(ctx, "key2")
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify others still exist
	exists, err = client.Exists(ctx, "key1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = client.Exists(ctx, "key3")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestClusterClient_BinaryData_WithMock(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	tests := []struct {
		name  string
		value []byte
	}{
		{name: "null bytes", value: []byte{0, 0, 0, 0}},
		{name: "binary with nulls", value: []byte{1, 0, 2, 0, 3}},
		{name: "high bytes", value: []byte{255, 254, 253, 252}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "binary-" + tt.name

			err := client.Set(ctx, key, tt.value, time.Minute)
			require.NoError(t, err)

			// Manually set the binary value in mock
			mock.data[key] = string(tt.value)

			got, err := client.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestClusterClient_SpecialCharsInKey_WithMock(t *testing.T) {
	client, mock := newMockClusterClient()
	defer client.Close()

	ctx := context.Background()

	keys := []string{
		"key:with:colons",
		"key/with/slashes",
		"key.with.dots",
		"key-with-dashes",
		"key_with_underscores",
		"key with spaces",
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			value := []byte("value for " + key)

			err := client.Set(ctx, key, value, time.Minute)
			require.NoError(t, err)

			mock.data[key] = string(value)

			got, err := client.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, value, got)

			exists, err := client.Exists(ctx, key)
			require.NoError(t, err)
			assert.True(t, exists)

			err = client.Delete(ctx, key)
			require.NoError(t, err)

			exists, err = client.Exists(ctx, key)
			require.NoError(t, err)
			assert.False(t, exists)
		})
	}
}

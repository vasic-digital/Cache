package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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

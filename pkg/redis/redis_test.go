package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify configuration, construction, and interface
// compliance without requiring a live Redis instance.

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
	client := New(DefaultConfig())
	require.NotNil(t, client.Underlying())
	_ = client.Close()
}

func TestClient_Close(t *testing.T) {
	client := New(DefaultConfig())
	err := client.Close()
	assert.NoError(t, err)
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

// Interface compliance at compile time
var _ interface {
	Close() error
} = (*Client)(nil)

var _ interface {
	Close() error
} = (*ClusterClient)(nil)

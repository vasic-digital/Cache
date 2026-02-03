// Package redis provides a Redis-backed cache implementation of the
// Cache interface defined in digital.vasic.cache/pkg/cache.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config holds connection parameters for a single Redis instance.
type Config struct {
	// Addr is the host:port address (e.g. "localhost:6379").
	Addr string
	// Password for Redis AUTH. Empty string means no auth.
	Password string
	// DB selects the Redis database (0-15).
	DB int
	// PoolSize is the maximum number of socket connections.
	PoolSize int
	// MinIdleConns is the minimum number of idle connections.
	MinIdleConns int
	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration
	// ReadTimeout is the timeout for socket reads.
	ReadTimeout time.Duration
	// WriteTimeout is the timeout for socket writes.
	WriteTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults for local
// development.
func DefaultConfig() *Config {
	return &Config{
		Addr:         "localhost:6379",
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 2,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}
}

// ClusterConfig holds connection parameters for a Redis Cluster.
type ClusterConfig struct {
	// Addrs is a seed list of cluster node addresses.
	Addrs []string
	// Password for Redis AUTH.
	Password string
	// PoolSize per cluster node.
	PoolSize int
	// MinIdleConns per cluster node.
	MinIdleConns int
	// DialTimeout for new connections.
	DialTimeout time.Duration
	// ReadTimeout for socket reads.
	ReadTimeout time.Duration
	// WriteTimeout for socket writes.
	WriteTimeout time.Duration
}

// Client implements the cache.Cache interface using a single Redis
// instance via go-redis/v9.
type Client struct {
	rdb *redis.Client
}

// New creates a new Redis cache client from the provided Config.
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	return &Client{rdb: rdb}
}

// Get retrieves a value from Redis. Returns nil, nil on cache miss.
func (c *Client) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get %q: %w", key, err)
	}
	return data, nil
}

// Set stores a value in Redis with the given TTL. A zero TTL means
// the key will not expire.
func (c *Client) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

// Delete removes a key from Redis.
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis delete %q: %w", key, err)
	}
	return nil
}

// Exists reports whether the key is present in Redis.
func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis exists %q: %w", key, err)
	}
	return n > 0, nil
}

// Close closes the underlying Redis connection.
func (c *Client) Close() error {
	return c.rdb.Close()
}

// HealthCheck pings the Redis server and returns an error if it is
// unreachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis health check: %w", err)
	}
	return nil
}

// Underlying returns the raw go-redis client for advanced operations.
func (c *Client) Underlying() *redis.Client {
	return c.rdb
}

// NewCluster creates a Redis cache client backed by a Redis Cluster.
func NewCluster(cfg *ClusterConfig) *ClusterClient {
	if cfg == nil {
		cfg = &ClusterConfig{
			Addrs: []string{"localhost:7000", "localhost:7001", "localhost:7002"},
		}
	}

	rdb := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:        cfg.Addrs,
		Password:     cfg.Password,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	return &ClusterClient{rdb: rdb}
}

// clusterCmdable abstracts the Redis cluster command interface for testing.
type clusterCmdable interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	Ping(ctx context.Context) *redis.StatusCmd
	Close() error
}

// ClusterClient implements the cache.Cache interface using a Redis
// Cluster via go-redis/v9.
type ClusterClient struct {
	rdb clusterCmdable
}

// Get retrieves a value from the Redis Cluster.
func (c *ClusterClient) Get(ctx context.Context, key string) ([]byte, error) {
	data, err := c.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis cluster get %q: %w", key, err)
	}
	return data, nil
}

// Set stores a value in the Redis Cluster.
func (c *ClusterClient) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.rdb.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis cluster set %q: %w", key, err)
	}
	return nil
}

// Delete removes a key from the Redis Cluster.
func (c *ClusterClient) Delete(ctx context.Context, key string) error {
	if err := c.rdb.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis cluster delete %q: %w", key, err)
	}
	return nil
}

// Exists reports whether a key exists in the Redis Cluster.
func (c *ClusterClient) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis cluster exists %q: %w", key, err)
	}
	return n > 0, nil
}

// Close closes the underlying Redis Cluster connection.
func (c *ClusterClient) Close() error {
	return c.rdb.Close()
}

// HealthCheck pings the Redis Cluster.
func (c *ClusterClient) HealthCheck(ctx context.Context) error {
	if err := c.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis cluster health check: %w", err)
	}
	return nil
}

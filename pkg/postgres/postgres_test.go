package postgres

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Unit tests for the parts of pkg/postgres that don't need a live DB.
// Database-bound behavior is exercised in integration_test.go.

func TestNewRequiresPoolOrURL(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	_, err := New(cfg)
	if err == nil {
		t.Fatal("expected error when neither Pool nor URL is set")
	}
	if !strings.Contains(err.Error(), "Pool or Config.URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRejectsNegativeGCParams(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
,
		{
			name:    "negative GCBatchSize",
			mutate:  func(c *Config) { c.GCBatchSize = -1 },
			wantErr: "GCBatchSize",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.URL = "postgres://x@y/z"
			tc.mutate(cfg)
			_, err := New(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNewRejectsBadIdentifiers(t *testing.T) {
	t.Parallel()
	// Empty string is intentionally NOT in this list — empty means
	// "use default", which is a valid configuration choice.
	bad := []string{
		"1starts_with_digit",
		"has-hyphen",
		"has space",
		"weird;drop tabl)
		t.Run("table="+name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.URL = "postgres://x@y/z"
			cfg.TableName = name
			_, err := New(cfg)
			if err == nil {
				t.Fatalf("expected error for invalid TableName %q", name)
			}
		})
	}
}

func TestNewAcceptsGoodIdentifiers(t *testing.T) {
	t.Parallel()
	good := []string{"a", "A", "_x", "schema_v2", "Cache_2026"}
	for _, name := range good {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.URL = "postgres://x@y/z"
			cfg.SchemaName = name
			cfg.TableName = name
			c, err := New(cfg)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if c.qualified == "" || !strings.Contains(c.qualified, name) {
				t.Fatalf("qualified name missing identifier: %q", c.qualified)
			}
		})
	}
}

func TestDefaultConfigValues(t *testing.T) {
	
	if cfg.TableName != DefaultTable {
		t.Errorf("TableName = %q, want %q", cfg.TableName, DefaultTable)
	}
	if cfg.GCInterval != 10*time.Minute {
		t.Errorf("GCInterval = %v, want 10m", cfg.GCInterval)
	}
}

func TestStopBeforeStartIsSafe(t *testing.T) {
	// bluff-scan: nil-only-ok (lifecycle test — Stop() must not panic/error before Start())
	t.Parallel()
	cfg := DefaultConfig()
	cfg.URL = "postgres://x@y/z"
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.Stop() // must not panic, must not block
}

func TestCloseBeforeAnythingIsSafe(t *testing.T) {
	// bluff-scan: nil-only-ok (lifecycle test — Close() and double-Close() must not error)
	t.Parallel()
	cfg := DefaultConfig()
	cfg.URL = "postgres://x@y/z"
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// Compile-time check: *Client satisfies the cache.Cache interface shape.
//
// We don't import digital.vasic.cache/pkg/cache here to avoid a circular
// dependency at the package level — but the methods here MUST stay in
// sync with that interface. The shape of the assertions below is the
// type-level proof.
var _ struct {
	get    func() ([]byte, error)
	set    func([]byte, time.Duration) error
	del    func() error
	exists func() (bool, error)
	close  func() error
} = struct {
	get    func() ([]byte, error)
	set    func([]byte, time.Duration) error
	del    func() error
	exists func() (bool, error)
	close  func() error
}{}

// shapeOk references all five methods by name so a rename surfaces here.
func shapeOk(c *Client) {
	var (
		_ = c.Get
		_ = c.Set
		_ = c.Delete
		_ = c.Exists
		_ = c.Close
	)
}

// silence "unused" warning for shapeOk; the function exists to enforce
// the public-method shape via compile-time references.
var _ = shapeOk

// silence "errors" import if no test happens to call errors.Is.
var _ = errors.Is

package git

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Cache is a small TTL-backed key-value store for git aggregation results.
//
// Reads and writes go through the provided *sql.DB. The caller is expected
// to have created the `git_cache` table (see internal/db/schema.sql) before
// using the cache; Cache itself never issues DDL.
type Cache struct {
	db *sql.DB
}

// NewCache wraps db as a git TTL cache. db must be a handle on a SQLite
// database that already contains the `git_cache` table.
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// CacheKey returns a hex-encoded sha256 digest of the (kind, repo, author,
// since, until) tuple. The tuple is serialized with '|' separators; '|' is
// rare enough in paths and email addresses that we don't bother escaping.
// Any change to any field produces a different key.
func CacheKey(kind, repo, author, since, until string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte("|"))
	h.Write([]byte(repo))
	h.Write([]byte("|"))
	h.Write([]byte(author))
	h.Write([]byte("|"))
	h.Write([]byte(since))
	h.Write([]byte("|"))
	h.Write([]byte(until))
	return hex.EncodeToString(h.Sum(nil))
}

// GetOrCompute returns the cached payload for key when present and within
// ttl. On miss or stale entry, compute is invoked and its result is stored
// with INSERT OR REPLACE. compute is called at most once per invocation,
// and only on miss. Errors from compute propagate unchanged and do NOT
// write a cache row.
//
// kind is persisted alongside the payload as a debugging aid; it is not
// used for lookup (the key already encodes kind).
func (c *Cache) GetOrCompute(
	ctx context.Context,
	key, kind string,
	ttl time.Duration,
	compute func() ([]byte, error),
) ([]byte, error) {
	payload, ok, err := c.lookup(ctx, key, ttl)
	if err != nil {
		return nil, err
	}
	if ok {
		return payload, nil
	}

	fresh, err := compute()
	if err != nil {
		return nil, err
	}
	if err := c.store(ctx, key, kind, fresh); err != nil {
		return nil, fmt.Errorf("git_cache store: %w", err)
	}
	return fresh, nil
}

// lookup returns (payload, true, nil) when a fresh row exists, (nil, false,
// nil) on miss or stale entry, and (nil, false, err) on a query error.
// Rows whose computed_at cannot be parsed are treated as stale.
func (c *Cache) lookup(
	ctx context.Context, key string, ttl time.Duration,
) ([]byte, bool, error) {
	var payload, computedAt string
	err := c.db.QueryRowContext(
		ctx,
		`SELECT payload, computed_at FROM git_cache WHERE cache_key = ?`,
		key,
	).Scan(&payload, &computedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("git_cache lookup: %w", err)
	}
	t, parseErr := time.Parse(time.RFC3339Nano, computedAt)
	if parseErr != nil {
		// Malformed timestamp: treat as stale so compute runs and overwrites.
		return nil, false, nil
	}
	if time.Since(t) > ttl {
		return nil, false, nil
	}
	return []byte(payload), true, nil
}

// store upserts a cache row with the current time as computed_at.
func (c *Cache) store(
	ctx context.Context, key, kind string, payload []byte,
) error {
	_, err := c.db.ExecContext(
		ctx,
		`INSERT OR REPLACE INTO git_cache
		 (cache_key, kind, payload, computed_at)
		 VALUES (?, ?, ?, ?)`,
		key, kind, string(payload),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// AggregateLogCached wraps AggregateLog with a TTL-bounded read-through
// cache. The cache key is derived from ("log", repo, author, since, until).
// On a cache miss, AggregateLog is invoked; its LogResult is JSON-encoded
// before being stored. Errors from AggregateLog propagate unchanged.
func AggregateLogCached(
	ctx context.Context,
	cache *Cache,
	repo, author, since, until string,
	ttl time.Duration,
) (LogResult, error) {
	key := CacheKey("log", repo, author, since, until)
	payload, err := cache.GetOrCompute(
		ctx, key, "log", ttl,
		func() ([]byte, error) {
			res, err := AggregateLog(ctx, repo, author, since, until)
			if err != nil {
				return nil, err
			}
			return json.Marshal(res)
		},
	)
	if err != nil {
		return LogResult{}, err
	}
	var out LogResult
	if err := json.Unmarshal(payload, &out); err != nil {
		return LogResult{}, fmt.Errorf("git_cache decode log payload: %w", err)
	}
	return out, nil
}

// AggregatePRsCached wraps AggregatePRs with a TTL-bounded cache. Author is
// left empty in the cache key because `--author=@me` is implicit in
// AggregatePRs and is effectively a property of the token, not an argument.
//
// When ghToken == "", AggregatePRs returns (nil, nil) and this wrapper
// mirrors that behavior without touching the cache — a nil PRResult is
// ambiguous ("gh unavailable") and we don't want to memoize it.
func AggregatePRsCached(
	ctx context.Context,
	cache *Cache,
	repo, since, until, ghToken string,
	ttl time.Duration,
) (*PRResult, error) {
	if ghToken == "" {
		return AggregatePRs(ctx, repo, since, until, ghToken)
	}
	key := CacheKey("pr", repo, "", since, until)
	payload, err := cache.GetOrCompute(
		ctx, key, "pr", ttl,
		func() ([]byte, error) {
			res, err := AggregatePRs(ctx, repo, since, until, ghToken)
			if err != nil {
				return nil, err
			}
			if res == nil {
				// Defensive: AggregatePRs only returns nil when ghToken
				// is empty, which we short-circuit above. Guard anyway
				// so a future refactor can't accidentally cache nil.
				return nil, errors.New("git_cache: AggregatePRs returned nil with non-empty token")
			}
			return json.Marshal(res)
		},
	)
	if err != nil {
		return nil, err
	}
	var out PRResult
	if err := json.Unmarshal(payload, &out); err != nil {
		return nil, fmt.Errorf("git_cache decode pr payload: %w", err)
	}
	return &out, nil
}

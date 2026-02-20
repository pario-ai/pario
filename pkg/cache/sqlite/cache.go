package sqlite

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/pario-ai/pario/pkg/models"
)

// Cache is an exact-match prompt cache backed by SQLite.
type Cache struct {
	db      *sql.DB
	ttl     time.Duration
	hits    atomic.Int64
	misses  atomic.Int64
}

const createCacheTable = `
CREATE TABLE IF NOT EXISTS cache_entries (
	prompt_hash TEXT NOT NULL,
	model TEXT NOT NULL,
	response BLOB NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	ttl_seconds INTEGER NOT NULL,
	PRIMARY KEY (prompt_hash, model)
);
`

// New creates a Cache with the given database path and default TTL.
func New(dbPath string, ttl time.Duration) (*Cache, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open cache db: %w", err)
	}

	if _, err := db.Exec(createCacheTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate cache db: %w", err)
	}

	return &Cache{db: db, ttl: ttl}, nil
}

// HashPrompt computes a SHA-256 hash of the model and messages.
func HashPrompt(model string, messages []models.ChatMessage) string {
	h := sha256.New()
	h.Write([]byte(model))
	data, _ := json.Marshal(messages)
	h.Write(data)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Get retrieves a cached response. Returns nil if not found or expired.
func (c *Cache) Get(promptHash, model string) ([]byte, bool) {
	var response []byte
	var createdAt time.Time
	var ttlSeconds int64

	err := c.db.QueryRow(
		`SELECT response, created_at, ttl_seconds FROM cache_entries WHERE prompt_hash = ? AND model = ?`,
		promptHash, model,
	).Scan(&response, &createdAt, &ttlSeconds)

	if err != nil {
		c.misses.Add(1)
		return nil, false
	}

	ttl := time.Duration(ttlSeconds) * time.Second
	if time.Since(createdAt) > ttl {
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	return response, true
}

// Put stores a response in the cache.
func (c *Cache) Put(promptHash, model string, response []byte) error {
	_, err := c.db.Exec(
		`INSERT OR REPLACE INTO cache_entries (prompt_hash, model, response, created_at, ttl_seconds)
		 VALUES (?, ?, ?, ?, ?)`,
		promptHash, model, response, time.Now().UTC(), int64(c.ttl.Seconds()),
	)
	if err != nil {
		return fmt.Errorf("cache put: %w", err)
	}
	return nil
}

// Stats returns cache performance metrics.
func (c *Cache) Stats() (models.CacheStats, error) {
	var count int64
	err := c.db.QueryRow(`SELECT COUNT(*) FROM cache_entries`).Scan(&count)
	if err != nil {
		return models.CacheStats{}, fmt.Errorf("cache stats: %w", err)
	}
	return models.CacheStats{
		Entries: count,
		Hits:    c.hits.Load(),
		Misses:  c.misses.Load(),
	}, nil
}

// Clear removes cache entries. If expiredOnly is true, only expired entries are removed.
func (c *Cache) Clear(expiredOnly bool) error {
	var query string
	if expiredOnly {
		query = `DELETE FROM cache_entries WHERE (julianday('now') - julianday(created_at)) * 86400 > ttl_seconds`
	} else {
		query = `DELETE FROM cache_entries`
	}
	_, err := c.db.Exec(query)
	if err != nil {
		return fmt.Errorf("cache clear: %w", err)
	}
	return nil
}

// Close releases the database connection.
func (c *Cache) Close() error {
	return c.db.Close()
}

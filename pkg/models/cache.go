package models

import "time"

// CacheEntry stores a cached LLM response.
type CacheEntry struct {
	PromptHash string    `json:"prompt_hash"`
	Model      string    `json:"model"`
	Response   []byte    `json:"response"`
	CreatedAt  time.Time `json:"created_at"`
	TTL        time.Duration `json:"ttl"`
}

// CacheStats reports cache performance metrics.
type CacheStats struct {
	Entries int64 `json:"entries"`
	Hits    int64 `json:"hits"`
	Misses  int64 `json:"misses"`
}

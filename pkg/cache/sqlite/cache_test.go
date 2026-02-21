package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/pario-ai/pario/pkg/models"
)

func newTestCache(t *testing.T, ttl time.Duration) *Cache {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "cache_test.db")
	c, err := New(dbPath, ttl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestHashPrompt(t *testing.T) {
	msgs := []models.ChatMessage{{Role: "user", Content: "hello"}}
	h1 := HashPrompt("gpt-4", msgs)
	h2 := HashPrompt("gpt-4", msgs)
	h3 := HashPrompt("gpt-3.5-turbo", msgs)

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different model should produce different hash")
	}
}

func TestPutAndGet(t *testing.T) {
	c := newTestCache(t, time.Hour)
	hash := HashPrompt("gpt-4", []models.ChatMessage{{Role: "user", Content: "hi"}})

	if err := c.Put(hash, "gpt-4", []byte(`{"response":"hello"}`)); err != nil {
		t.Fatal(err)
	}

	data, ok := c.Get(hash, "gpt-4")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(data) != `{"response":"hello"}` {
		t.Errorf("unexpected response: %s", data)
	}

	// Miss for different model
	_, ok = c.Get(hash, "gpt-3.5-turbo")
	if ok {
		t.Error("expected cache miss for different model")
	}
}

func TestTTLExpiration(t *testing.T) {
	c := newTestCache(t, 1*time.Millisecond)
	hash := "testhash"

	if err := c.Put(hash, "gpt-4", []byte("data")); err != nil {
		t.Fatal(err)
	}

	time.Sleep(10 * time.Millisecond)

	_, ok := c.Get(hash, "gpt-4")
	if ok {
		t.Error("expected cache miss after TTL expiration")
	}
}

func TestStats(t *testing.T) {
	c := newTestCache(t, time.Hour)

	_ = c.Put("h1", "gpt-4", []byte("data"))
	c.Get("h1", "gpt-4") // hit
	c.Get("h2", "gpt-4") // miss

	stats, err := c.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Entries != 1 {
		t.Errorf("expected 1 entry, got %d", stats.Entries)
	}
	if stats.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestClear(t *testing.T) {
	c := newTestCache(t, time.Hour)

	_ = c.Put("h1", "gpt-4", []byte("data"))
	_ = c.Put("h2", "gpt-4", []byte("data"))

	if err := c.Clear(false); err != nil {
		t.Fatal(err)
	}

	stats, _ := c.Stats()
	if stats.Entries != 0 {
		t.Errorf("expected 0 entries after clear, got %d", stats.Entries)
	}
}

package tracker

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/pario-ai/pario/pkg/models"
)

// Tracker records and queries token usage.
type Tracker interface {
	// Record stores a usage record.
	Record(ctx context.Context, rec models.UsageRecord) error
	// QueryByKey returns usage records for an API key since a given time.
	QueryByKey(ctx context.Context, apiKey string, since time.Time) ([]models.UsageRecord, error)
	// TotalByKey returns total tokens used by an API key since a given time.
	TotalByKey(ctx context.Context, apiKey string, since time.Time) (int64, error)
	// TotalByKeyAndModel returns total tokens used by an API key and model since a given time.
	TotalByKeyAndModel(ctx context.Context, apiKey, model string, since time.Time) (int64, error)
	// Summary returns aggregated usage summaries, optionally filtered by API key.
	Summary(ctx context.Context, apiKey string) ([]models.UsageSummary, error)
	// ResolveSession returns a session ID for the given API key, using the explicit
	// session ID if provided, otherwise auto-detecting by time gap.
	ResolveSession(ctx context.Context, apiKey, explicitID string, gapTimeout time.Duration) (string, error)
	// ListSessions returns all sessions, optionally filtered by API key.
	ListSessions(ctx context.Context, apiKey string) ([]models.Session, error)
	// SessionRequests returns per-request detail for a session with context growth.
	SessionRequests(ctx context.Context, sessionID string) ([]models.SessionRequest, error)
	// Close releases resources.
	Close() error
}

// SQLiteTracker implements Tracker with a SQLite database.
type SQLiteTracker struct {
	db *sql.DB
}

const createTable = `
CREATE TABLE IF NOT EXISTS usage_records (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	api_key TEXT NOT NULL,
	model TEXT NOT NULL,
	prompt_tokens INTEGER NOT NULL,
	completion_tokens INTEGER NOT NULL,
	total_tokens INTEGER NOT NULL,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_usage_key_time ON usage_records(api_key, created_at);
`

const createSessionsTable = `
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	api_key TEXT NOT NULL,
	started_at DATETIME NOT NULL,
	last_activity DATETIME NOT NULL,
	request_count INTEGER NOT NULL DEFAULT 0,
	total_tokens INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_key ON sessions(api_key);
`

// New creates a SQLiteTracker and runs auto-migration.
func New(dbPath string) (*SQLiteTracker, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open tracker db: %w", err)
	}

	if _, err := db.Exec(createTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate tracker db: %w", err)
	}

	if _, err := db.Exec(createSessionsTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sessions table: %w", err)
	}

	// Add session_id column to usage_records if missing.
	if !columnExists(db, "usage_records", "session_id") {
		if _, err := db.Exec(`ALTER TABLE usage_records ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`); err != nil {
			db.Close()
			return nil, fmt.Errorf("add session_id column: %w", err)
		}
	}

	return &SQLiteTracker{db: db}, nil
}

func columnExists(db *sql.DB, table, column string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

// generateSessionID creates a session ID like sess_20260221_a3f9c2.
func generateSessionID() string {
	b := make([]byte, 3)
	rand.Read(b)
	return fmt.Sprintf("sess_%s_%s", time.Now().UTC().Format("20060102"), hex.EncodeToString(b))
}

// Record stores a usage record and updates session counters.
func (t *SQLiteTracker) Record(ctx context.Context, rec models.UsageRecord) error {
	_, err := t.db.ExecContext(ctx,
		`INSERT INTO usage_records (api_key, model, session_id, prompt_tokens, completion_tokens, total_tokens, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.APIKey, rec.Model, rec.SessionID, rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens, rec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}

	// Update session counters if session is set.
	if rec.SessionID != "" {
		_, err = t.db.ExecContext(ctx,
			`UPDATE sessions SET last_activity = ?, request_count = request_count + 1, total_tokens = total_tokens + ? WHERE id = ?`,
			rec.CreatedAt, rec.TotalTokens, rec.SessionID,
		)
		if err != nil {
			return fmt.Errorf("update session counters: %w", err)
		}
	}

	return nil
}

// ResolveSession returns a session ID. If explicitID is non-empty, it ensures
// the session row exists and returns it. Otherwise it finds the most recent
// session for the API key and reuses it if within gapTimeout, or creates a new one.
func (t *SQLiteTracker) ResolveSession(ctx context.Context, apiKey, explicitID string, gapTimeout time.Duration) (string, error) {
	now := time.Now().UTC()

	if explicitID != "" {
		_, err := t.db.ExecContext(ctx,
			`INSERT INTO sessions (id, api_key, started_at, last_activity) VALUES (?, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			explicitID, apiKey, now, now,
		)
		if err != nil {
			return "", fmt.Errorf("ensure session: %w", err)
		}
		return explicitID, nil
	}

	// Auto-detect: find most recent session for this key.
	var lastID string
	var lastActivity time.Time
	err := t.db.QueryRowContext(ctx,
		`SELECT id, last_activity FROM sessions WHERE api_key = ? ORDER BY last_activity DESC LIMIT 1`,
		apiKey,
	).Scan(&lastID, &lastActivity)

	if err == nil && now.Sub(lastActivity) <= gapTimeout {
		return lastID, nil
	}

	// Create new session.
	newID := generateSessionID()
	_, err = t.db.ExecContext(ctx,
		`INSERT INTO sessions (id, api_key, started_at, last_activity) VALUES (?, ?, ?, ?)`,
		newID, apiKey, now, now,
	)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return newID, nil
}

// ListSessions returns all sessions, optionally filtered by API key.
func (t *SQLiteTracker) ListSessions(ctx context.Context, apiKey string) ([]models.Session, error) {
	query := `SELECT id, api_key, started_at, last_activity, request_count, total_tokens FROM sessions`
	var args []any
	if apiKey != "" {
		query += ` WHERE api_key = ?`
		args = append(args, apiKey)
	}
	query += ` ORDER BY started_at DESC`

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []models.Session
	for rows.Next() {
		var s models.Session
		if err := rows.Scan(&s.ID, &s.APIKey, &s.StartedAt, &s.LastActivity, &s.RequestCount, &s.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// SessionRequests returns per-request detail for a session with context growth.
func (t *SQLiteTracker) SessionRequests(ctx context.Context, sessionID string) ([]models.SessionRequest, error) {
	rows, err := t.db.QueryContext(ctx,
		`SELECT created_at, prompt_tokens, completion_tokens, total_tokens
		 FROM usage_records WHERE session_id = ? ORDER BY created_at ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("session requests: %w", err)
	}
	defer rows.Close()

	var reqs []models.SessionRequest
	var prevPrompt int
	seq := 0
	for rows.Next() {
		var r models.SessionRequest
		if err := rows.Scan(&r.CreatedAt, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan session request: %w", err)
		}
		seq++
		r.Seq = seq
		if seq > 1 {
			r.ContextGrowth = r.PromptTokens - prevPrompt
		}
		prevPrompt = r.PromptTokens
		reqs = append(reqs, r)
	}
	return reqs, rows.Err()
}

// QueryByKey returns usage records for an API key since a given time.
func (t *SQLiteTracker) QueryByKey(ctx context.Context, apiKey string, since time.Time) ([]models.UsageRecord, error) {
	rows, err := t.db.QueryContext(ctx,
		`SELECT id, api_key, model, session_id, prompt_tokens, completion_tokens, total_tokens, created_at
		 FROM usage_records WHERE api_key = ? AND created_at >= ? ORDER BY created_at DESC`,
		apiKey, since,
	)
	if err != nil {
		return nil, fmt.Errorf("query usage: %w", err)
	}
	defer rows.Close()

	var records []models.UsageRecord
	for rows.Next() {
		var r models.UsageRecord
		if err := rows.Scan(&r.ID, &r.APIKey, &r.Model, &r.SessionID, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan usage: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// TotalByKey returns total tokens used by an API key since a given time.
func (t *SQLiteTracker) TotalByKey(ctx context.Context, apiKey string, since time.Time) (int64, error) {
	var total int64
	err := t.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_tokens), 0) FROM usage_records WHERE api_key = ? AND created_at >= ?`,
		apiKey, since,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total usage: %w", err)
	}
	return total, nil
}

// TotalByKeyAndModel returns total tokens used by an API key and model since a given time.
func (t *SQLiteTracker) TotalByKeyAndModel(ctx context.Context, apiKey, model string, since time.Time) (int64, error) {
	var total int64
	err := t.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(total_tokens), 0) FROM usage_records WHERE api_key = ? AND model = ? AND created_at >= ?`,
		apiKey, model, since,
	).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("total usage by model: %w", err)
	}
	return total, nil
}

// Summary returns aggregated usage grouped by API key and model.
func (t *SQLiteTracker) Summary(ctx context.Context, apiKey string) ([]models.UsageSummary, error) {
	query := `SELECT api_key, model, COUNT(*), SUM(prompt_tokens), SUM(completion_tokens), SUM(total_tokens)
		 FROM usage_records`
	var args []any
	if apiKey != "" {
		query += ` WHERE api_key = ?`
		args = append(args, apiKey)
	}
	query += ` GROUP BY api_key, model ORDER BY api_key, model`

	rows, err := t.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("summary: %w", err)
	}
	defer rows.Close()

	var summaries []models.UsageSummary
	for rows.Next() {
		var s models.UsageSummary
		if err := rows.Scan(&s.APIKey, &s.Model, &s.RequestCount, &s.TotalPrompt, &s.TotalCompletion, &s.TotalTokens); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

// Close releases the database connection.
func (t *SQLiteTracker) Close() error {
	return t.db.Close()
}

package tracker

import (
	"context"
	"database/sql"
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
	// Summary returns aggregated usage summaries, optionally filtered by API key.
	Summary(ctx context.Context, apiKey string) ([]models.UsageSummary, error)
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

	return &SQLiteTracker{db: db}, nil
}

// Record stores a usage record.
func (t *SQLiteTracker) Record(ctx context.Context, rec models.UsageRecord) error {
	_, err := t.db.ExecContext(ctx,
		`INSERT INTO usage_records (api_key, model, prompt_tokens, completion_tokens, total_tokens, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		rec.APIKey, rec.Model, rec.PromptTokens, rec.CompletionTokens, rec.TotalTokens, rec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("record usage: %w", err)
	}
	return nil
}

// QueryByKey returns usage records for an API key since a given time.
func (t *SQLiteTracker) QueryByKey(ctx context.Context, apiKey string, since time.Time) ([]models.UsageRecord, error) {
	rows, err := t.db.QueryContext(ctx,
		`SELECT id, api_key, model, prompt_tokens, completion_tokens, total_tokens, created_at
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
		if err := rows.Scan(&r.ID, &r.APIKey, &r.Model, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &r.CreatedAt); err != nil {
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

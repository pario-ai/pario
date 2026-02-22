package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pario-ai/pario/pkg/models"
	_ "modernc.org/sqlite"
)

// Logger writes and queries audit entries in a dedicated SQLite database.
type Logger struct {
	db     *sql.DB
	cfg    models.AuditConfig
	done   chan struct{}
	wg     sync.WaitGroup
	include map[string]bool
	exclude map[string]bool
}

// New opens the audit SQLite database and creates the schema.
func New(cfg models.AuditConfig) (*Logger, error) {
	db, err := sql.Open("sqlite", cfg.DBPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open audit db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate audit db: %w", err)
	}

	inc := make(map[string]bool)
	for _, v := range cfg.Include {
		inc[v] = true
	}
	exc := make(map[string]bool)
	for _, v := range cfg.ExcludeModels {
		exc[v] = true
	}

	l := &Logger{
		db:      db,
		cfg:     cfg,
		done:    make(chan struct{}),
		include: inc,
		exclude: exc,
	}

	l.wg.Add(1)
	go l.retentionLoop()

	return l, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS audit_log (
		request_id     TEXT PRIMARY KEY,
		api_key_hash   TEXT NOT NULL,
		api_key_prefix TEXT NOT NULL,
		model          TEXT NOT NULL,
		session_id     TEXT,
		provider       TEXT,
		request_body   TEXT,
		response_body  TEXT,
		request_headers TEXT,
		status_code    INTEGER,
		prompt_tokens  INTEGER,
		completion_tokens INTEGER,
		total_tokens   INTEGER,
		latency_ms     INTEGER,
		created_at     DATETIME NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_model ON audit_log(model)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_created ON audit_log(created_at)`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_prefix ON audit_log(api_key_prefix)`)
	return err
}

// Log inserts an audit entry, respecting include/exclude configuration.
func (l *Logger) Log(ctx context.Context, entry models.AuditEntry) error {
	if l == nil || l.db == nil {
		return nil
	}
	if l.exclude[entry.Model] {
		return nil
	}

	reqBody := entry.RequestBody
	respBody := entry.ResponseBody
	var headersJSON string

	if !l.include["prompts"] {
		reqBody = ""
	}
	if !l.include["responses"] {
		respBody = ""
	}
	if l.include["metadata"] && entry.RequestHeaders != nil {
		b, _ := json.Marshal(entry.RequestHeaders)
		headersJSON = string(b)
	}

	if l.cfg.MaxBodySize > 0 {
		if len(reqBody) > l.cfg.MaxBodySize {
			reqBody = reqBody[:l.cfg.MaxBodySize]
		}
		if len(respBody) > l.cfg.MaxBodySize {
			respBody = respBody[:l.cfg.MaxBodySize]
		}
	}

	_, err := l.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO audit_log
		(request_id, api_key_hash, api_key_prefix, model, session_id, provider,
		 request_body, response_body, request_headers, status_code,
		 prompt_tokens, completion_tokens, total_tokens, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.RequestID, entry.APIKeyHash, entry.APIKeyPrefix,
		entry.Model, entry.SessionID, entry.Provider,
		reqBody, respBody, headersJSON, entry.StatusCode,
		entry.PromptTokens, entry.CompletionTokens, entry.TotalTokens,
		entry.LatencyMs, entry.CreatedAt,
	)
	return err
}

// Query returns audit entries matching the given options.
func (l *Logger) Query(ctx context.Context, opts models.AuditQueryOpts) ([]models.AuditEntry, error) {
	q := `SELECT request_id, api_key_hash, api_key_prefix, model, session_id, provider,
		request_body, response_body, request_headers, status_code,
		prompt_tokens, completion_tokens, total_tokens, latency_ms, created_at
		FROM audit_log WHERE 1=1`
	var args []any

	if opts.RequestID != "" {
		q += " AND request_id = ?"
		args = append(args, opts.RequestID)
	}
	if opts.Model != "" {
		q += " AND model = ?"
		args = append(args, opts.Model)
	}
	if !opts.Since.IsZero() {
		q += " AND created_at >= ?"
		args = append(args, opts.Since)
	}
	if opts.APIKeyPrefix != "" {
		q += " AND api_key_prefix = ?"
		args = append(args, opts.APIKeyPrefix)
	}
	if opts.SessionID != "" {
		q += " AND session_id = ?"
		args = append(args, opts.SessionID)
	}

	q += " ORDER BY created_at DESC"

	limit := opts.Limit
	if limit <= 0 {
		limit = 100
	}
	q += " LIMIT ?"
	args = append(args, limit)

	rows, err := l.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query audit: %w", err)
	}
	defer rows.Close()

	var entries []models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		var headers sql.NullString
		var sessionID sql.NullString
		var provider sql.NullString
		if err := rows.Scan(
			&e.RequestID, &e.APIKeyHash, &e.APIKeyPrefix, &e.Model,
			&sessionID, &provider,
			&e.RequestBody, &e.ResponseBody, &headers, &e.StatusCode,
			&e.PromptTokens, &e.CompletionTokens, &e.TotalTokens,
			&e.LatencyMs, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit row: %w", err)
		}
		e.SessionID = sessionID.String
		e.Provider = provider.String
		if headers.Valid && headers.String != "" {
			_ = json.Unmarshal([]byte(headers.String), &e.RequestHeaders)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Stats returns aggregate counts grouped by model and day.
func (l *Logger) Stats(ctx context.Context) ([]models.AuditStat, error) {
	rows, err := l.db.QueryContext(ctx,
		`SELECT model, date(created_at) as day, count(*) as cnt
		 FROM audit_log GROUP BY model, day ORDER BY day DESC, model`)
	if err != nil {
		return nil, fmt.Errorf("audit stats: %w", err)
	}
	defer rows.Close()

	var stats []models.AuditStat
	for rows.Next() {
		var s models.AuditStat
		var day sql.NullString
		if err := rows.Scan(&s.Model, &day, &s.Count); err != nil {
			return nil, fmt.Errorf("scan audit stat: %w", err)
		}
		s.Day = day.String
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// Cleanup deletes entries older than the configured retention period.
func (l *Logger) Cleanup(ctx context.Context) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -l.cfg.RetentionDays)
	res, err := l.db.ExecContext(ctx,
		`DELETE FROM audit_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("audit cleanup: %w", err)
	}
	return res.RowsAffected()
}

// Close stops the retention goroutine and closes the database.
func (l *Logger) Close() error {
	close(l.done)
	l.wg.Wait()
	return l.db.Close()
}

func (l *Logger) retentionLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-l.done:
			return
		case <-ticker.C:
			_, _ = l.Cleanup(context.Background())
		}
	}
}

// HashAPIKey returns the SHA-256 hex hash and 8-char prefix for an API key.
func HashAPIKey(key string) (hash, prefix string) {
	h := sha256.Sum256([]byte(key))
	hash = hex.EncodeToString(h[:])
	if len(key) > 8 {
		prefix = key[:8]
	} else {
		prefix = key
	}
	return hash, prefix
}

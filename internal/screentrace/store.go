package screentrace

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"baize/internal/sqliteutil"
)

type Store struct {
	path     string
	db       *sql.DB
	initOnce sync.Once
	initErr  error
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) AddRecord(ctx context.Context, record Record) (Record, error) {
	if err := s.ensureReady(); err != nil {
		return Record{}, err
	}
	if record.ID == "" {
		record.ID = newID()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.CapturedAt.IsZero() {
		record.CapturedAt = record.CreatedAt
	}
	visibleJSON, err := json.Marshal(record.VisibleText)
	if err != nil {
		return Record{}, err
	}
	appsJSON, err := json.Marshal(record.Apps)
	if err != nil {
		return Record{}, err
	}
	keywordsJSON, err := json.Marshal(record.Keywords)
	if err != nil {
		return Record{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO screentrace_records (
			id, captured_at, image_path, image_hash, width, height, display_index,
			scene_summary, visible_text_json, apps_json, task_guess, keywords_json,
			sensitive_level, confidence, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.ID,
		formatSQLiteTime(record.CapturedAt),
		record.ImagePath,
		record.ImageHash,
		record.Width,
		record.Height,
		record.DisplayIndex,
		record.SceneSummary,
		string(visibleJSON),
		string(appsJSON),
		record.TaskGuess,
		string(keywordsJSON),
		record.SensitiveLevel,
		record.Confidence,
		formatSQLiteTime(record.CreatedAt),
	)
	if err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Store) LatestRecord(ctx context.Context) (Record, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Record{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, captured_at, image_path, image_hash, width, height, display_index,
			scene_summary, visible_text_json, apps_json, task_guess, keywords_json,
			sensitive_level, confidence, created_at
		FROM screentrace_records
		ORDER BY captured_at DESC, id DESC
		LIMIT 1
	`)
	record, ok, err := scanRecord(row)
	return record, ok, err
}

func (s *Store) ListRecentRecords(ctx context.Context, limit int) ([]Record, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, captured_at, image_path, image_hash, width, height, display_index,
			scene_summary, visible_text_json, apps_json, task_guess, keywords_json,
			sensitive_level, confidence, created_at
		FROM screentrace_records
		ORDER BY captured_at DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		record, _, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		if !record.CapturedAt.IsZero() || record.ID != "" {
			out = append(out, record)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListRecordsBetween(ctx context.Context, start, end time.Time) ([]Record, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, captured_at, image_path, image_hash, width, height, display_index,
			scene_summary, visible_text_json, apps_json, task_guess, keywords_json,
			sensitive_level, confidence, created_at
		FROM screentrace_records
		WHERE captured_at >= ? AND captured_at < ?
		ORDER BY captured_at ASC, id ASC
	`, formatSQLiteTime(start), formatSQLiteTime(end))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		record, _, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpsertDigest(ctx context.Context, digest Digest) (Digest, error) {
	if err := s.ensureReady(); err != nil {
		return Digest{}, err
	}
	now := time.Now().UTC()
	if digest.ID == "" {
		existing, ok, err := s.GetDigestByBucket(ctx, digest.BucketStart)
		if err != nil {
			return Digest{}, err
		}
		if ok {
			digest.ID = existing.ID
			digest.CreatedAt = existing.CreatedAt
			if !digest.WrittenToKB {
				digest.WrittenToKB = existing.WrittenToKB
			}
			if digest.KnowledgeEntryID == "" {
				digest.KnowledgeEntryID = existing.KnowledgeEntryID
			}
		}
	}
	if digest.ID == "" {
		digest.ID = newID()
	}
	if digest.CreatedAt.IsZero() {
		digest.CreatedAt = now
	}
	digest.UpdatedAt = now

	keywordsJSON, err := json.Marshal(digest.Keywords)
	if err != nil {
		return Digest{}, err
	}
	appsJSON, err := json.Marshal(digest.DominantApps)
	if err != nil {
		return Digest{}, err
	}
	tasksJSON, err := json.Marshal(digest.DominantTasks)
	if err != nil {
		return Digest{}, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO screentrace_digests (
			id, bucket_start, bucket_end, record_count, summary, keywords_json,
			dominant_apps_json, dominant_tasks_json, written_to_kb, knowledge_entry_id,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(bucket_start) DO UPDATE SET
			bucket_end = excluded.bucket_end,
			record_count = excluded.record_count,
			summary = excluded.summary,
			keywords_json = excluded.keywords_json,
			dominant_apps_json = excluded.dominant_apps_json,
			dominant_tasks_json = excluded.dominant_tasks_json,
			written_to_kb = excluded.written_to_kb,
			knowledge_entry_id = excluded.knowledge_entry_id,
			updated_at = excluded.updated_at
	`,
		digest.ID,
		formatSQLiteTime(digest.BucketStart),
		formatSQLiteTime(digest.BucketEnd),
		digest.RecordCount,
		digest.Summary,
		string(keywordsJSON),
		string(appsJSON),
		string(tasksJSON),
		boolToInt(digest.WrittenToKB),
		digest.KnowledgeEntryID,
		formatSQLiteTime(digest.CreatedAt),
		formatSQLiteTime(digest.UpdatedAt),
	)
	if err != nil {
		return Digest{}, err
	}
	return digest, nil
}

func (s *Store) GetDigestByBucket(ctx context.Context, bucketStart time.Time) (Digest, bool, error) {
	if err := s.ensureReady(); err != nil {
		return Digest{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT
			id, bucket_start, bucket_end, record_count, summary, keywords_json,
			dominant_apps_json, dominant_tasks_json, written_to_kb, knowledge_entry_id,
			created_at, updated_at
		FROM screentrace_digests
		WHERE bucket_start = ?
	`, formatSQLiteTime(bucketStart))
	return scanDigest(row)
}

func (s *Store) ListRecentDigests(ctx context.Context, limit int) ([]Digest, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			id, bucket_start, bucket_end, record_count, summary, keywords_json,
			dominant_apps_json, dominant_tasks_json, written_to_kb, knowledge_entry_id,
			created_at, updated_at
		FROM screentrace_digests
		ORDER BY bucket_start DESC, id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Digest
	for rows.Next() {
		digest, _, err := scanDigest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, digest)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CountRecords(ctx context.Context) (int, error) {
	if err := s.ensureReady(); err != nil {
		return 0, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM screentrace_records`)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) DeleteRecordsBefore(ctx context.Context, cutoff time.Time) ([]string, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT image_path FROM screentrace_records WHERE captured_at < ?`, formatSQLiteTime(cutoff))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if path != "" {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM screentrace_records WHERE captured_at < ?`, formatSQLiteTime(cutoff)); err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM screentrace_digests WHERE bucket_end < ?`, formatSQLiteTime(cutoff)); err != nil {
		return nil, err
	}
	return paths, nil
}

func (s *Store) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS screentrace_records (
				id TEXT PRIMARY KEY,
				captured_at TEXT NOT NULL,
				image_path TEXT NOT NULL,
				image_hash TEXT NOT NULL,
				width INTEGER NOT NULL DEFAULT 0,
				height INTEGER NOT NULL DEFAULT 0,
				display_index INTEGER NOT NULL DEFAULT 0,
				scene_summary TEXT NOT NULL DEFAULT '',
				visible_text_json TEXT NOT NULL DEFAULT '[]',
				apps_json TEXT NOT NULL DEFAULT '[]',
				task_guess TEXT NOT NULL DEFAULT '',
				keywords_json TEXT NOT NULL DEFAULT '[]',
				sensitive_level TEXT NOT NULL DEFAULT 'low',
				confidence REAL NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS screentrace_records_captured_idx
				ON screentrace_records (captured_at DESC, id DESC);
			CREATE TABLE IF NOT EXISTS screentrace_digests (
				id TEXT PRIMARY KEY,
				bucket_start TEXT NOT NULL UNIQUE,
				bucket_end TEXT NOT NULL,
				record_count INTEGER NOT NULL DEFAULT 0,
				summary TEXT NOT NULL DEFAULT '',
				keywords_json TEXT NOT NULL DEFAULT '[]',
				dominant_apps_json TEXT NOT NULL DEFAULT '[]',
				dominant_tasks_json TEXT NOT NULL DEFAULT '[]',
				written_to_kb INTEGER NOT NULL DEFAULT 0,
				knowledge_entry_id TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS screentrace_digests_bucket_idx
				ON screentrace_digests (bucket_start DESC, id DESC);
		`)
	})
	return s.initErr
}

func scanRecord(scanner interface{ Scan(...any) error }) (Record, bool, error) {
	var (
		record          Record
		capturedAt      string
		visibleTextJSON string
		appsJSON        string
		keywordsJSON    string
		createdAt       string
	)
	err := scanner.Scan(
		&record.ID,
		&capturedAt,
		&record.ImagePath,
		&record.ImageHash,
		&record.Width,
		&record.Height,
		&record.DisplayIndex,
		&record.SceneSummary,
		&visibleTextJSON,
		&appsJSON,
		&record.TaskGuess,
		&keywordsJSON,
		&record.SensitiveLevel,
		&record.Confidence,
		&createdAt,
	)
	if err == sql.ErrNoRows {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	if err := json.Unmarshal([]byte(visibleTextJSON), &record.VisibleText); err != nil {
		return Record{}, false, err
	}
	if err := json.Unmarshal([]byte(appsJSON), &record.Apps); err != nil {
		return Record{}, false, err
	}
	if err := json.Unmarshal([]byte(keywordsJSON), &record.Keywords); err != nil {
		return Record{}, false, err
	}
	if record.CapturedAt, err = parseSQLiteTime(capturedAt); err != nil {
		return Record{}, false, err
	}
	if record.CreatedAt, err = parseSQLiteTime(createdAt); err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func scanDigest(scanner interface{ Scan(...any) error }) (Digest, bool, error) {
	var (
		digest       Digest
		bucketStart  string
		bucketEnd    string
		keywordsJSON string
		appsJSON     string
		tasksJSON    string
		writtenToKB  int
		createdAt    string
		updatedAt    string
	)
	err := scanner.Scan(
		&digest.ID,
		&bucketStart,
		&bucketEnd,
		&digest.RecordCount,
		&digest.Summary,
		&keywordsJSON,
		&appsJSON,
		&tasksJSON,
		&writtenToKB,
		&digest.KnowledgeEntryID,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return Digest{}, false, nil
	}
	if err != nil {
		return Digest{}, false, err
	}
	if err := json.Unmarshal([]byte(keywordsJSON), &digest.Keywords); err != nil {
		return Digest{}, false, err
	}
	if err := json.Unmarshal([]byte(appsJSON), &digest.DominantApps); err != nil {
		return Digest{}, false, err
	}
	if err := json.Unmarshal([]byte(tasksJSON), &digest.DominantTasks); err != nil {
		return Digest{}, false, err
	}
	digest.WrittenToKB = writtenToKB == 1
	var errParse error
	if digest.BucketStart, errParse = parseSQLiteTime(bucketStart); errParse != nil {
		return Digest{}, false, errParse
	}
	if digest.BucketEnd, errParse = parseSQLiteTime(bucketEnd); errParse != nil {
		return Digest{}, false, errParse
	}
	if digest.CreatedAt, errParse = parseSQLiteTime(createdAt); errParse != nil {
		return Digest{}, false, errParse
	}
	if digest.UpdatedAt, errParse = parseSQLiteTime(updatedAt); errParse != nil {
		return Digest{}, false, errParse
	}
	return digest, true, nil
}

func formatSQLiteTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UTC().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}

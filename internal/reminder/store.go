package reminder

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"sync"
	"time"

	"baize/internal/sqliteutil"
)

type Frequency string

const (
	FrequencyOnce  Frequency = "once"
	FrequencyDaily Frequency = "daily"
)

type Target struct {
	Interface string `json:"interface"`
	UserID    string `json:"user_id"`
}

type Reminder struct {
	ID          string    `json:"id"`
	Target      Target    `json:"target"`
	Message     string    `json:"message"`
	Frequency   Frequency `json:"frequency"`
	NextRunAt   time.Time `json:"next_run_at"`
	DailyHour   int       `json:"daily_hour,omitempty"`
	DailyMinute int       `json:"daily_minute,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store struct {
	path     string
	db       *sql.DB
	initOnce sync.Once
	initErr  error
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) List(ctx context.Context) ([]Reminder, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, target_interface, target_user_id, message, frequency, next_run_at, daily_hour, daily_minute, created_at, updated_at
		FROM reminders
		ORDER BY next_run_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Reminder
	for rows.Next() {
		item, err := scanReminder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) SaveAll(ctx context.Context, items []Reminder) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	return sqliteutil.WithTx(ctx, s.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM reminders`); err != nil {
			return err
		}
		for _, item := range items {
			if err := insertReminder(ctx, tx, item); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Add(ctx context.Context, item Reminder) (Reminder, error) {
	if err := s.ensureReady(); err != nil {
		return Reminder{}, err
	}

	if item.ID == "" {
		item.ID = newID()
	}
	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	if err := insertReminder(ctx, s.db, item); err != nil {
		return Reminder{}, err
	}
	return item, nil
}

func (s *Store) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS reminders (
				id TEXT PRIMARY KEY,
				target_interface TEXT NOT NULL DEFAULT '',
				target_user_id TEXT NOT NULL DEFAULT '',
				message TEXT NOT NULL,
				frequency TEXT NOT NULL,
				next_run_at TEXT NOT NULL,
				daily_hour INTEGER NOT NULL DEFAULT 0,
				daily_minute INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			);
			CREATE INDEX IF NOT EXISTS reminders_target_next_run_idx
				ON reminders (target_interface, target_user_id, next_run_at);
		`)
	})
	return s.initErr
}

type reminderExec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func insertReminder(ctx context.Context, exec reminderExec, item Reminder) error {
	_, err := exec.ExecContext(ctx, `
		INSERT INTO reminders (
			id, target_interface, target_user_id, message, frequency,
			next_run_at, daily_hour, daily_minute, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.Target.Interface, item.Target.UserID, item.Message, item.Frequency,
		formatSQLiteTime(item.NextRunAt), item.DailyHour, item.DailyMinute,
		formatSQLiteTime(item.CreatedAt), formatSQLiteTime(item.UpdatedAt))
	return err
}

func scanReminder(scanner interface{ Scan(...any) error }) (Reminder, error) {
	var (
		item            Reminder
		nextRunAt       string
		createdAt       string
		updatedAt       string
		targetInterface string
		targetUserID    string
	)
	if err := scanner.Scan(
		&item.ID,
		&targetInterface,
		&targetUserID,
		&item.Message,
		&item.Frequency,
		&nextRunAt,
		&item.DailyHour,
		&item.DailyMinute,
		&createdAt,
		&updatedAt,
	); err != nil {
		return Reminder{}, err
	}
	item.Target = Target{Interface: targetInterface, UserID: targetUserID}
	var err error
	if item.NextRunAt, err = parseSQLiteTime(nextRunAt); err != nil {
		return Reminder{}, err
	}
	if item.CreatedAt, err = parseSQLiteTime(createdAt); err != nil {
		return Reminder{}, err
	}
	if item.UpdatedAt, err = parseSQLiteTime(updatedAt); err != nil {
		return Reminder{}, err
	}
	return item, nil
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

func newID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().Format("20060102150405")
	}
	return hex.EncodeToString(buf[:])
}

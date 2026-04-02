package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	appsvc "baize/internal/app"
	"baize/internal/screentrace"
	"baize/internal/sqliteutil"
)

const desktopSettingsRowID = "primary"

type desktopSettingsStore struct {
	path     string
	db       *sql.DB
	initOnce sync.Once
	initErr  error
}

type desktopSettingsFile struct {
	WeixinHistoryMessages       int               `json:"weixin_history_messages"`
	WeixinHistoryRunes          int               `json:"weixin_history_runes"`
	WeixinEverythingPath        string            `json:"weixin_everything_path"`
	DisabledToolNames           []string          `json:"disabled_tool_names,omitempty"`
	DesktopChatSessions         map[string]string `json:"desktop_chat_sessions,omitempty"`
	ScreenTraceEnabled          bool              `json:"screentrace_enabled"`
	ScreenTraceIntervalSeconds  int               `json:"screentrace_interval_seconds"`
	ScreenTraceRetentionDays    int               `json:"screentrace_retention_days"`
	ScreenTraceVisionProfileID  string            `json:"screentrace_vision_profile_id"`
	ScreenTraceWriteDigestsToKB bool              `json:"screentrace_write_digests_to_kb"`
}

func newDesktopSettingsStore(dataDir string) *desktopSettingsStore {
	return &desktopSettingsStore{
		path: filepath.Join(dataDir, "app.db"),
	}
}

func (s *desktopSettingsStore) Load() (desktopSettingsFile, bool, error) {
	if s == nil || s.path == "" {
		return desktopSettingsFile{}, false, nil
	}
	if err := s.ensureReady(); err != nil {
		return desktopSettingsFile{}, false, err
	}

	row := s.db.QueryRowContext(context.Background(), `
		SELECT
			weixin_history_messages, weixin_history_runes, weixin_everything_path,
			disabled_tool_names_json, desktop_chat_sessions_json,
			screentrace_enabled, screentrace_interval_seconds, screentrace_retention_days,
			screentrace_vision_profile_id, screentrace_write_digests_to_kb
		FROM desktop_settings
		WHERE id = ?
	`, desktopSettingsRowID)
	var (
		cfg                desktopSettingsFile
		disabledToolsJSON  string
		chatSessionsJSON   string
		screenTraceEnabled int
		screenTraceWriteKB int
	)
	if err := row.Scan(
		&cfg.WeixinHistoryMessages,
		&cfg.WeixinHistoryRunes,
		&cfg.WeixinEverythingPath,
		&disabledToolsJSON,
		&chatSessionsJSON,
		&screenTraceEnabled,
		&cfg.ScreenTraceIntervalSeconds,
		&cfg.ScreenTraceRetentionDays,
		&cfg.ScreenTraceVisionProfileID,
		&screenTraceWriteKB,
	); err != nil {
		if err == sql.ErrNoRows {
			return desktopSettingsFile{}, false, nil
		}
		return desktopSettingsFile{}, false, err
	}
	cfg.ScreenTraceEnabled = screenTraceEnabled == 1
	cfg.ScreenTraceWriteDigestsToKB = screenTraceWriteKB == 1
	if err := json.Unmarshal([]byte(strings.TrimSpace(disabledToolsJSON)), &cfg.DisabledToolNames); err != nil {
		return desktopSettingsFile{}, false, err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(chatSessionsJSON)), &cfg.DesktopChatSessions); err != nil {
		return desktopSettingsFile{}, false, err
	}
	normalizeDesktopSettings(&cfg)
	return cfg, true, nil
}

func (s *desktopSettingsStore) Save(cfg desktopSettingsFile) error {
	if s == nil || s.path == "" {
		return nil
	}
	if err := s.ensureReady(); err != nil {
		return err
	}

	normalizeDesktopSettings(&cfg)
	disabledToolsJSON, err := json.Marshal(cfg.DisabledToolNames)
	if err != nil {
		return err
	}
	chatSessionsJSON, err := json.Marshal(cfg.DesktopChatSessions)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(context.Background(), `
		INSERT INTO desktop_settings (
			id, weixin_history_messages, weixin_history_runes, weixin_everything_path,
			disabled_tool_names_json, desktop_chat_sessions_json,
			screentrace_enabled, screentrace_interval_seconds, screentrace_retention_days,
			screentrace_vision_profile_id, screentrace_write_digests_to_kb
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			weixin_history_messages = excluded.weixin_history_messages,
			weixin_history_runes = excluded.weixin_history_runes,
			weixin_everything_path = excluded.weixin_everything_path,
			disabled_tool_names_json = excluded.disabled_tool_names_json,
			desktop_chat_sessions_json = excluded.desktop_chat_sessions_json,
			screentrace_enabled = excluded.screentrace_enabled,
			screentrace_interval_seconds = excluded.screentrace_interval_seconds,
			screentrace_retention_days = excluded.screentrace_retention_days,
			screentrace_vision_profile_id = excluded.screentrace_vision_profile_id,
			screentrace_write_digests_to_kb = excluded.screentrace_write_digests_to_kb
	`,
		desktopSettingsRowID,
		cfg.WeixinHistoryMessages,
		cfg.WeixinHistoryRunes,
		cfg.WeixinEverythingPath,
		string(disabledToolsJSON),
		string(chatSessionsJSON),
		boolToDBInt(cfg.ScreenTraceEnabled),
		cfg.ScreenTraceIntervalSeconds,
		cfg.ScreenTraceRetentionDays,
		cfg.ScreenTraceVisionProfileID,
		boolToDBInt(cfg.ScreenTraceWriteDigestsToKB),
	)
	return err
}

func (s *desktopSettingsStore) ensureReady() error {
	s.initOnce.Do(func() {
		s.db, s.initErr = sqliteutil.Open(s.path)
		if s.initErr != nil {
			return
		}
		_, s.initErr = s.db.Exec(`
			CREATE TABLE IF NOT EXISTS desktop_settings (
				id TEXT PRIMARY KEY,
				weixin_history_messages INTEGER NOT NULL DEFAULT 0,
				weixin_history_runes INTEGER NOT NULL DEFAULT 0,
				weixin_everything_path TEXT NOT NULL DEFAULT '',
				disabled_tool_names_json TEXT NOT NULL DEFAULT '[]',
				desktop_chat_sessions_json TEXT NOT NULL DEFAULT '{}',
				screentrace_enabled INTEGER NOT NULL DEFAULT 0,
				screentrace_interval_seconds INTEGER NOT NULL DEFAULT 15,
				screentrace_retention_days INTEGER NOT NULL DEFAULT 7,
				screentrace_vision_profile_id TEXT NOT NULL DEFAULT '',
				screentrace_write_digests_to_kb INTEGER NOT NULL DEFAULT 0
			)
		`)
		if s.initErr != nil {
			return
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN disabled_tool_names_json TEXT NOT NULL DEFAULT '[]'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN desktop_chat_sessions_json TEXT NOT NULL DEFAULT '{}'`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN screentrace_enabled INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN screentrace_interval_seconds INTEGER NOT NULL DEFAULT 15`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN screentrace_retention_days INTEGER NOT NULL DEFAULT 7`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN screentrace_vision_profile_id TEXT NOT NULL DEFAULT ''`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
		if _, err := s.db.Exec(`ALTER TABLE desktop_settings ADD COLUMN screentrace_write_digests_to_kb INTEGER NOT NULL DEFAULT 0`); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
			s.initErr = err
		}
	})
	return s.initErr
}

func normalizeDesktopSettings(cfg *desktopSettingsFile) {
	if cfg == nil {
		return
	}
	if cfg.WeixinHistoryMessages < 0 {
		cfg.WeixinHistoryMessages = 0
	}
	if cfg.WeixinHistoryRunes < 0 {
		cfg.WeixinHistoryRunes = 0
	}
	cfg.WeixinEverythingPath = filepath.Clean(strings.TrimSpace(cfg.WeixinEverythingPath))
	if cfg.WeixinEverythingPath == "." {
		cfg.WeixinEverythingPath = ""
	}
	screenTraceDefaults := screentrace.DefaultSettings()
	if cfg.ScreenTraceIntervalSeconds <= 0 {
		cfg.ScreenTraceIntervalSeconds = screenTraceDefaults.IntervalSeconds
	}
	if cfg.ScreenTraceRetentionDays <= 0 {
		cfg.ScreenTraceRetentionDays = screenTraceDefaults.RetentionDays
	}
	cfg.ScreenTraceVisionProfileID = strings.TrimSpace(cfg.ScreenTraceVisionProfileID)
	cfg.DisabledToolNames = appsvc.NormalizeAgentToolNames(cfg.DisabledToolNames)
	cfg.DesktopChatSessions = normalizeDesktopChatSessions(cfg.DesktopChatSessions)
}

func normalizeDesktopChatSessions(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for project, sessionID := range raw {
		project = strings.ToLower(strings.TrimSpace(project))
		sessionID = strings.TrimSpace(sessionID)
		if project == "" || sessionID == "" {
			continue
		}
		out[project] = sessionID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func boolToDBInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

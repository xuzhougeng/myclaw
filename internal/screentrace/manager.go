package screentrace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"baize/internal/ai"
	"baize/internal/modelconfig"
)

const disabledPollInterval = 5 * time.Second

type Analyzer interface {
	AnalyzeScreenImage(ctx context.Context, cfg modelconfig.Config, fileName, imageURL string) (ai.ScreenAnalysis, error)
	SummarizeScreenDigest(ctx context.Context, cfg modelconfig.Config, records []ai.ScreenDigestRecord) (ai.ScreenDigestSummary, error)
}

type ModelConfigSource interface {
	Get(ctx context.Context, id string) (modelconfig.Config, bool, error)
}

type DigestRecorder func(context.Context, Digest) (string, error)

type ManagerOptions struct {
	Capture             captureFunc
	Now                 func() time.Time
	DigestRecorder      DigestRecorder
	SimilarityThreshold int
}

type Manager struct {
	dataDir      string
	store        *Store
	analyzer     Analyzer
	modelSource  ModelConfigSource
	capture      captureFunc
	now          func() time.Time
	recordDigest DigestRecorder

	mu                  sync.RWMutex
	settings            Settings
	running             bool
	cancel              context.CancelFunc
	wakeCh              chan struct{}
	lastCaptureAt       time.Time
	lastAnalysisAt      time.Time
	lastDigestAt        time.Time
	lastError           string
	lastImagePath       string
	totalRecords        int
	skippedDuplicates   int
	lastCleanupAt       time.Time
	similarityThreshold int
}

func NewManager(dataDir string, store *Store, analyzer Analyzer, modelSource ModelConfigSource, opts ManagerOptions) *Manager {
	settings := DefaultSettings()
	manager := &Manager{
		dataDir:             strings.TrimSpace(dataDir),
		store:               store,
		analyzer:            analyzer,
		modelSource:         modelSource,
		capture:             opts.Capture,
		now:                 opts.Now,
		recordDigest:        opts.DigestRecorder,
		settings:            settings,
		wakeCh:              make(chan struct{}, 1),
		similarityThreshold: opts.SimilarityThreshold,
	}
	if manager.capture == nil {
		manager.capture = capturePrimaryDisplay
	}
	if manager.now == nil {
		manager.now = time.Now
	}
	if manager.similarityThreshold <= 0 {
		manager.similarityThreshold = DefaultSimilarityThreshold
	}
	if store != nil {
		if count, err := store.CountRecords(context.Background()); err == nil {
			manager.totalRecords = count
		}
	}
	return manager
}

func (m *Manager) Start() {
	if m == nil {
		return
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.running = true
	m.cancel = cancel
	m.mu.Unlock()

	go m.run(ctx)
}

func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.running = false
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) SetSettings(settings Settings) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.settings = settings.Normalize()
	m.mu.Unlock()
	m.signalWake()
}

func (m *Manager) Settings() Settings {
	if m == nil {
		return DefaultSettings()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

func (m *Manager) Status() Status {
	if m == nil {
		return Status{Settings: DefaultSettings()}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Status{
		Settings:          m.settings,
		Running:           m.running,
		LastCaptureAt:     m.lastCaptureAt,
		LastAnalysisAt:    m.lastAnalysisAt,
		LastDigestAt:      m.lastDigestAt,
		LastError:         m.lastError,
		LastImagePath:     m.lastImagePath,
		TotalRecords:      m.totalRecords,
		SkippedDuplicates: m.skippedDuplicates,
	}
}

func (m *Manager) CaptureNow(ctx context.Context) error {
	if m == nil {
		return fmt.Errorf("screentrace manager is nil")
	}
	settings := m.Settings()
	if !settings.Enabled {
		return fmt.Errorf("screentrace is disabled")
	}
	return m.captureOnce(ctx, settings)
}

func (m *Manager) ListRecentRecords(ctx context.Context, limit int) ([]Record, error) {
	if m == nil || m.store == nil {
		return nil, nil
	}
	return m.store.ListRecentRecords(ctx, limit)
}

func (m *Manager) ListRecentDigests(ctx context.Context, limit int) ([]Digest, error) {
	if m == nil || m.store == nil {
		return nil, nil
	}
	return m.store.ListRecentDigests(ctx, limit)
}

func (m *Manager) run(ctx context.Context) {
	if settings := m.Settings(); settings.Enabled {
		_ = m.captureOnce(ctx, settings)
	}
	for {
		settings := m.Settings()
		wait := disabledPollInterval
		if settings.Enabled {
			wait = time.Duration(settings.IntervalSeconds) * time.Second
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-m.wakeCh:
			timer.Stop()
			continue
		case <-timer.C:
			if !settings.Enabled {
				continue
			}
			_ = m.captureOnce(ctx, settings)
		}
	}
}

func (m *Manager) captureOnce(ctx context.Context, settings Settings) error {
	settings = settings.Normalize()
	if m.store == nil || m.analyzer == nil || m.modelSource == nil {
		return m.setError(fmt.Errorf("screentrace dependencies are not configured"))
	}
	if strings.TrimSpace(settings.VisionProfileID) == "" {
		return m.setError(fmt.Errorf("screentrace vision profile is not configured"))
	}

	cfg, ok, err := m.modelSource.Get(ctx, settings.VisionProfileID)
	if err != nil {
		return m.setError(err)
	}
	if !ok {
		return m.setError(fmt.Errorf("screentrace vision profile %q was not found", settings.VisionProfileID))
	}

	capturedAt := m.now().UTC()
	capture, err := m.capture(ctx)
	if err != nil {
		return m.setError(err)
	}
	m.updateLastCapture(capturedAt)

	latest, found, err := m.store.LatestRecord(ctx)
	if err != nil {
		return m.setError(err)
	}
	if found && hashDistance(latest.ImageHash, capture.ImageHash) <= m.similarityThreshold {
		m.incrementDuplicate()
		return nil
	}

	recordID := newID()
	imagePath := screenshotFilePath(m.dataDir, capturedAt, recordID)
	if err := os.MkdirAll(filepath.Dir(imagePath), 0o755); err != nil {
		return m.setError(err)
	}
	if err := os.WriteFile(imagePath, capture.ImageBytes, 0o644); err != nil {
		return m.setError(err)
	}

	analysis, err := m.analyzer.AnalyzeScreenImage(ctx, cfg, filepath.Base(imagePath), jpegDataURL(capture.ImageBytes))
	if err != nil {
		_ = os.Remove(imagePath)
		return m.setError(err)
	}

	record, err := m.store.AddRecord(ctx, Record{
		ID:             recordID,
		CapturedAt:     capturedAt,
		ImagePath:      imagePath,
		ImageHash:      capture.ImageHash,
		Width:          capture.Width,
		Height:         capture.Height,
		DisplayIndex:   capture.DisplayIndex,
		SceneSummary:   strings.TrimSpace(analysis.SceneSummary),
		VisibleText:    normalizeStrings(analysis.VisibleText),
		Apps:           normalizeStrings(analysis.Apps),
		TaskGuess:      strings.TrimSpace(analysis.TaskGuess),
		Keywords:       normalizeStrings(analysis.Keywords),
		SensitiveLevel: strings.TrimSpace(analysis.SensitiveLevel),
		Confidence:     analysis.Confidence,
	})
	if err != nil {
		_ = os.Remove(imagePath)
		return m.setError(err)
	}
	m.recordStored(record)

	if err := m.refreshDigest(ctx, cfg, settings, record.CapturedAt); err != nil {
		return m.setError(err)
	}
	if err := m.cleanupOldRecords(ctx, settings); err != nil {
		return m.setError(err)
	}
	m.clearError()
	return nil
}

func (m *Manager) refreshDigest(ctx context.Context, cfg modelconfig.Config, settings Settings, capturedAt time.Time) error {
	bucketStart, bucketEnd := digestWindow(capturedAt, settings.DigestIntervalMins)
	records, err := m.store.ListRecordsBetween(ctx, bucketStart, bucketEnd)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}

	inputs := make([]ai.ScreenDigestRecord, 0, len(records))
	for _, record := range records {
		inputs = append(inputs, ai.ScreenDigestRecord{
			CapturedAt:   record.CapturedAt.Local().Format("2006-01-02 15:04:05"),
			SceneSummary: record.SceneSummary,
			VisibleText:  record.VisibleText,
			Apps:         record.Apps,
			TaskGuess:    record.TaskGuess,
			Keywords:     record.Keywords,
		})
	}
	summary, err := m.analyzer.SummarizeScreenDigest(ctx, cfg, inputs)
	if err != nil {
		return err
	}
	existing, ok, err := m.store.GetDigestByBucket(ctx, bucketStart)
	if err != nil {
		return err
	}

	digest := Digest{
		BucketStart:      bucketStart,
		BucketEnd:        bucketEnd,
		RecordCount:      len(records),
		Summary:          strings.TrimSpace(summary.Summary),
		Keywords:         normalizeStrings(summary.Keywords),
		DominantApps:     normalizeStrings(summary.DominantApps),
		DominantTasks:    normalizeStrings(summary.DominantTasks),
		WrittenToKB:      ok && existing.WrittenToKB,
		KnowledgeEntryID: "",
	}
	if ok {
		digest.ID = existing.ID
		digest.CreatedAt = existing.CreatedAt
		digest.KnowledgeEntryID = existing.KnowledgeEntryID
	}

	now := m.now().UTC()
	if _, err := m.store.UpsertDigest(ctx, digest); err != nil {
		return err
	}
	if err := m.syncEligibleDigests(ctx, settings, bucketStart); err != nil {
		return err
	}
	m.updateLastDigest(now)
	return nil
}

func (m *Manager) syncEligibleDigests(ctx context.Context, settings Settings, currentBucketStart time.Time) error {
	if !settings.WriteDigestsToKB || m.recordDigest == nil {
		return nil
	}
	digests, err := m.store.ListRecentDigests(ctx, 16)
	if err != nil {
		return err
	}
	for _, digest := range digests {
		if digest.WrittenToKB {
			continue
		}
		if digest.BucketEnd.After(currentBucketStart) {
			continue
		}
		knowledgeID, err := m.recordDigest(ctx, digest)
		if err != nil {
			return err
		}
		digest.WrittenToKB = strings.TrimSpace(knowledgeID) != ""
		digest.KnowledgeEntryID = strings.TrimSpace(knowledgeID)
		if _, err := m.store.UpsertDigest(ctx, digest); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) cleanupOldRecords(ctx context.Context, settings Settings) error {
	if settings.RetentionDays <= 0 {
		return nil
	}
	now := m.now().UTC()
	m.mu.RLock()
	lastCleanupAt := m.lastCleanupAt
	m.mu.RUnlock()
	if !lastCleanupAt.IsZero() && now.Sub(lastCleanupAt) < 6*time.Hour {
		return nil
	}

	cutoff := now.Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
	paths, err := m.store.DeleteRecordsBefore(ctx, cutoff)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		_ = os.Remove(path)
	}
	count, err := m.store.CountRecords(ctx)
	if err != nil {
		return err
	}

	m.mu.Lock()
	m.totalRecords = count
	m.lastCleanupAt = now
	m.mu.Unlock()
	return nil
}

func (m *Manager) setError(err error) error {
	if err == nil {
		return nil
	}
	m.mu.Lock()
	m.lastError = strings.TrimSpace(err.Error())
	m.mu.Unlock()
	return err
}

func (m *Manager) clearError() {
	m.mu.Lock()
	m.lastError = ""
	m.mu.Unlock()
}

func (m *Manager) updateLastCapture(at time.Time) {
	m.mu.Lock()
	m.lastCaptureAt = at
	m.mu.Unlock()
}

func (m *Manager) recordStored(record Record) {
	m.mu.Lock()
	m.lastAnalysisAt = m.now().UTC()
	m.lastImagePath = record.ImagePath
	m.totalRecords++
	m.lastError = ""
	m.mu.Unlock()
}

func (m *Manager) updateLastDigest(at time.Time) {
	m.mu.Lock()
	m.lastDigestAt = at
	m.mu.Unlock()
}

func (m *Manager) incrementDuplicate() {
	m.mu.Lock()
	m.skippedDuplicates++
	m.mu.Unlock()
}

func (m *Manager) signalWake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

func digestWindow(at time.Time, intervalMinutes int) (time.Time, time.Time) {
	if intervalMinutes <= 0 {
		intervalMinutes = DefaultDigestIntervalMinute
	}
	interval := time.Duration(intervalMinutes) * time.Minute
	start := at.UTC().Truncate(interval)
	return start, start.Add(interval)
}

func screenshotFilePath(dataDir string, capturedAt time.Time, recordID string) string {
	stamp := capturedAt.UTC()
	return filepath.Join(
		dataDir,
		"screentrace",
		stamp.Format("2006"),
		stamp.Format("01"),
		stamp.Format("02"),
		recordID+".jpg",
	)
}

func normalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

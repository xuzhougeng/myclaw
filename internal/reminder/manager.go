package reminder

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type Notifier interface {
	Notify(ctx context.Context, item Reminder) error
}

type NotifierFunc func(ctx context.Context, item Reminder) error

func (f NotifierFunc) Notify(ctx context.Context, item Reminder) error {
	return f(ctx, item)
}

type Manager struct {
	store     *Store
	mu        sync.RWMutex
	notifiers map[string]Notifier
	now       func() time.Time
}

func NewManager(store *Store) *Manager {
	return &Manager{
		store:     store,
		notifiers: make(map[string]Notifier),
		now:       time.Now,
	}
}

func (m *Manager) RegisterNotifier(target Target, notifier Notifier) {
	if notifier == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifiers[targetKey(target)] = notifier
}

func (m *Manager) ScheduleAfter(ctx context.Context, target Target, after time.Duration, message string) (Reminder, error) {
	if after <= 0 {
		return Reminder{}, fmt.Errorf("duration must be positive")
	}
	return m.store.Add(ctx, Reminder{
		Target:    target,
		Message:   strings.TrimSpace(message),
		Frequency: FrequencyOnce,
		NextRunAt: m.now().Add(after),
	})
}

func (m *Manager) ScheduleAt(ctx context.Context, target Target, at time.Time, message string) (Reminder, error) {
	if !at.After(m.now()) {
		return Reminder{}, fmt.Errorf("scheduled time must be in the future")
	}
	return m.store.Add(ctx, Reminder{
		Target:    target,
		Message:   strings.TrimSpace(message),
		Frequency: FrequencyOnce,
		NextRunAt: at,
	})
}

func (m *Manager) ScheduleDaily(ctx context.Context, target Target, hour, minute int, message string) (Reminder, error) {
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return Reminder{}, fmt.Errorf("invalid daily time")
	}
	nextRun := nextDailyOccurrence(m.now(), hour, minute)
	return m.store.Add(ctx, Reminder{
		Target:      target,
		Message:     strings.TrimSpace(message),
		Frequency:   FrequencyDaily,
		NextRunAt:   nextRun,
		DailyHour:   hour,
		DailyMinute: minute,
	})
}

func (m *Manager) List(ctx context.Context, target Target) ([]Reminder, error) {
	items, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(items))
	for _, item := range items {
		if item.Target == target {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *Manager) Remove(ctx context.Context, target Target, idOrPrefix string) (Reminder, bool, error) {
	items, err := m.store.List(ctx)
	if err != nil {
		return Reminder{}, false, err
	}
	idOrPrefix = strings.TrimSpace(idOrPrefix)
	var matched *Reminder
	filtered := make([]Reminder, 0, len(items))
	for _, item := range items {
		if item.Target == target && strings.HasPrefix(item.ID, idOrPrefix) {
			if matched != nil {
				return Reminder{}, false, fmt.Errorf("multiple reminders match prefix %q", idOrPrefix)
			}
			copy := item
			matched = &copy
			continue
		}
		filtered = append(filtered, item)
	}
	if matched == nil {
		return Reminder{}, false, nil
	}
	if err := m.store.SaveAll(ctx, filtered); err != nil {
		return Reminder{}, false, err
	}
	return *matched, true, nil
}

func (m *Manager) Run(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := m.runDue(ctx, m.now()); err != nil {
				log.Printf("[reminder] run due failed: %v", err)
			}
		}
	}
}

func (m *Manager) runDue(ctx context.Context, now time.Time) error {
	items, err := m.store.List(ctx)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	changed := false
	for index := range items {
		item := &items[index]
		if item.NextRunAt.After(now) {
			continue
		}

		notifier := m.notifierFor(item.Target)
		if notifier == nil {
			continue
		}
		if err := notifier.Notify(ctx, *item); err != nil {
			log.Printf("[reminder] notify failed for %s: %v", item.ID, err)
			continue
		}

		changed = true
		switch item.Frequency {
		case FrequencyDaily:
			item.NextRunAt = nextDailyOccurrence(now, item.DailyHour, item.DailyMinute)
			item.UpdatedAt = now
		default:
			items = append(items[:index], items[index+1:]...)
			index--
		}
	}

	if !changed {
		return nil
	}
	return m.store.SaveAll(ctx, items)
}

func (m *Manager) notifierFor(target Target) Notifier {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.notifiers[targetKey(target)]
}

func nextDailyOccurrence(base time.Time, hour, minute int) time.Time {
	next := time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, base.Location())
	if !next.After(base) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func targetKey(target Target) string {
	return target.Interface + ":" + target.UserID
}

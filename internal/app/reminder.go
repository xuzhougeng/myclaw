package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"baize/internal/reminder"
)

var (
	durationWordPattern    = regexp.MustCompile(`^(?:in\s+)?(\d+)\s*(m|min|mins|minute|minutes|h|hr|hrs|hour|hours|d|day|days)\s+(.+)$`)
	chineseAfterPattern    = regexp.MustCompile(`^(?:过)?([\d一二两三四五六七八九十百零〇]+)\s*(分钟|小时|天)后?\s+(.+)$`)
	chineseCompactPattern  = regexp.MustCompile(`^([\d一二两三四五六七八九十百零〇]+)\s*(分钟|小时|天)后\s+(.+)$`)
	dailyPattern           = regexp.MustCompile(`^(?:每天|daily)\s+(\d{1,2}:\d{2})\s+(.+)$`)
	dateTimePattern        = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:\s+(\d{1,2}:\d{2}))?\s+(.+)$`)
	tomorrowPattern        = regexp.MustCompile(`^(?:明天|tomorrow)\s+(\d{1,2}:\d{2})\s+(.+)$`)
	naturalAfterPattern    = regexp.MustCompile(`^(.+?后)\s*提醒我(?:\s+)?(.+)$`)
	naturalDailyPattern    = regexp.MustCompile(`^(?:请)?每天\s+(\d{1,2}:\d{2})\s*提醒我(?:\s+)?(.+)$`)
	naturalDatePattern     = regexp.MustCompile(`^(?:请)?(\d{4}-\d{2}-\d{2})(?:\s+(\d{1,2}:\d{2}))?\s*提醒我(?:\s+)?(.+)$`)
	naturalTomorrowPattern = regexp.MustCompile(`^(?:请)?明天\s+(\d{1,2}:\d{2})\s*提醒我(?:\s+)?(.+)$`)
)

type reminderBackend interface {
	ScheduleAfter(ctx context.Context, target reminder.Target, after time.Duration, message string) (reminder.Reminder, error)
	ScheduleAt(ctx context.Context, target reminder.Target, at time.Time, message string) (reminder.Reminder, error)
	ScheduleDaily(ctx context.Context, target reminder.Target, hour, minute int, message string) (reminder.Reminder, error)
	ListAll(ctx context.Context) ([]reminder.Reminder, error)
	List(ctx context.Context, target reminder.Target) ([]reminder.Reminder, error)
	RemoveAny(ctx context.Context, idOrPrefix string) (reminder.Reminder, bool, error)
	Remove(ctx context.Context, target reminder.Target, idOrPrefix string) (reminder.Reminder, bool, error)
}

func (s *Service) handleReminderCommand(ctx context.Context, mc MessageContext, input string) (string, error) {
	if s.reminders == nil {
		return "提醒功能未启用。", nil
	}

	fields := strings.Fields(input)
	if len(fields) < 2 {
		return reminderUsage(), nil
	}

	command := strings.ToLower(fields[1])
	target := reminderTargetForContext(mc)
	switch command {
	case "help":
		return reminderUsage(), nil
	case "list":
		items, err := s.ListVisibleReminders(ctx, mc)
		if err != nil {
			return "", err
		}
		return formatReminderListForContext(mc, items), nil
	case "remove", "delete", "rm":
		if len(fields) < 3 {
			return "用法: /notice remove <提醒ID前缀>", nil
		}
		item, ok, err := s.RemoveVisibleReminder(ctx, mc, fields[2])
		if err != nil {
			return "", err
		}
		if !ok {
			return fmt.Sprintf("没有找到提醒 %q。", fields[2]), nil
		}
		return fmt.Sprintf("已删除提醒 #%s\n内容: %s", shortID(item.ID), item.Message), nil
	}

	spec := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
	item, err := s.scheduleReminderSpec(ctx, target, spec)
	if err != nil {
		return "", err
	}
	return formatReminderCreated(item), nil
}

func (s *Service) tryHandleNaturalReminder(ctx context.Context, mc MessageContext, input string) (string, bool, error) {
	if s.reminders == nil {
		return "", false, nil
	}

	spec, ok := normalizeNaturalReminderSpec(input)
	if !ok {
		return "", false, nil
	}

	target := reminderTargetForContext(mc)
	item, err := s.scheduleReminderSpec(ctx, reminder.Target{
		Interface: target.Interface,
		UserID:    target.UserID,
	}, spec)
	if err != nil {
		return "", true, err
	}
	return formatReminderCreated(item), true, nil
}

func (s *Service) scheduleReminderSpec(ctx context.Context, target reminder.Target, spec string) (reminder.Reminder, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return reminder.Reminder{}, errors.New(reminderUsage())
	}

	if matches := dailyPattern.FindStringSubmatch(spec); len(matches) == 3 {
		hour, minute, err := parseClock(matches[1])
		if err != nil {
			return reminder.Reminder{}, err
		}
		return s.reminders.ScheduleDaily(ctx, target, hour, minute, matches[2])
	}

	if matches := tomorrowPattern.FindStringSubmatch(spec); len(matches) == 3 {
		hour, minute, err := parseClock(matches[1])
		if err != nil {
			return reminder.Reminder{}, err
		}
		now := time.Now()
		at := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location()).Add(24 * time.Hour)
		return s.reminders.ScheduleAt(ctx, target, at, matches[2])
	}

	if matches := dateTimePattern.FindStringSubmatch(spec); len(matches) == 4 {
		clock := strings.TrimSpace(matches[2])
		if clock == "" {
			clock = "09:00"
		}
		at, err := time.ParseInLocation("2006-01-02 15:04", matches[1]+" "+clock, time.Local)
		if err != nil {
			return reminder.Reminder{}, fmt.Errorf("无法解析日期时间: %w", err)
		}
		return s.reminders.ScheduleAt(ctx, target, at, matches[3])
	}

	if matches := durationWordPattern.FindStringSubmatch(spec); len(matches) == 4 {
		after, err := parseEnglishDuration(matches[1], matches[2])
		if err != nil {
			return reminder.Reminder{}, err
		}
		return s.reminders.ScheduleAfter(ctx, target, after, matches[3])
	}

	if matches := chineseAfterPattern.FindStringSubmatch(spec); len(matches) == 4 {
		after, err := parseChineseDuration(matches[1], matches[2])
		if err != nil {
			return reminder.Reminder{}, err
		}
		return s.reminders.ScheduleAfter(ctx, target, after, matches[3])
	}

	if matches := chineseCompactPattern.FindStringSubmatch(spec); len(matches) == 4 {
		after, err := parseChineseDuration(matches[1], matches[2])
		if err != nil {
			return reminder.Reminder{}, err
		}
		return s.reminders.ScheduleAfter(ctx, target, after, matches[3])
	}

	return reminder.Reminder{}, fmt.Errorf("无法识别提醒语法。\n\n%s", reminderUsage())
}

func normalizeNaturalReminderSpec(input string) (string, bool) {
	text := strings.TrimSpace(input)
	if text == "" {
		return "", false
	}

	if matches := naturalDailyPattern.FindStringSubmatch(text); len(matches) == 3 {
		return "每天 " + strings.TrimSpace(matches[1]) + " " + strings.TrimSpace(matches[2]), true
	}

	if matches := naturalTomorrowPattern.FindStringSubmatch(text); len(matches) == 3 {
		return "明天 " + strings.TrimSpace(matches[1]) + " " + strings.TrimSpace(matches[2]), true
	}

	if matches := naturalDatePattern.FindStringSubmatch(text); len(matches) == 4 {
		spec := strings.TrimSpace(matches[1])
		if clock := strings.TrimSpace(matches[2]); clock != "" {
			spec += " " + clock
		}
		spec += " " + strings.TrimSpace(matches[3])
		return spec, true
	}

	if matches := naturalAfterPattern.FindStringSubmatch(text); len(matches) == 3 {
		return strings.TrimSpace(matches[1]) + " " + strings.TrimSpace(matches[2]), true
	}

	return "", false
}

func reminderUsage() string {
	return "用法:\n" +
		"/notice 2小时后 喝水\n" +
		"/notice 每天 09:00 写日报\n" +
		"/notice 2026-03-30 14:00 交房租\n" +
		"/notice 明天 09:30 开会\n" +
		"/notice list\n" +
		"/notice remove <提醒ID前缀>\n\n" +
		"`/cron` 与 `/notice` 等价。"
}

func formatReminderCreated(item reminder.Reminder) string {
	switch item.Frequency {
	case reminder.FrequencyDaily:
		return fmt.Sprintf("已创建每日提醒 #%s\n时间: 每天 %02d:%02d\n内容: %s",
			shortID(item.ID),
			item.DailyHour,
			item.DailyMinute,
			item.Message,
		)
	default:
		return fmt.Sprintf("已创建提醒 #%s\n时间: %s\n内容: %s",
			shortID(item.ID),
			item.NextRunAt.Local().Format("2006-01-02 15:04:05"),
			item.Message,
		)
	}
}

func formatReminderList(items []reminder.Reminder) string {
	return formatReminderListInternal(items, false)
}

func parseClock(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("无效时间格式 %q", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("无效小时 %q", parts[0])
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("无效分钟 %q", parts[1])
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("无效时间 %q", value)
	}
	return hour, minute, nil
}

func parseEnglishDuration(numText, unit string) (time.Duration, error) {
	value, err := strconv.Atoi(numText)
	if err != nil {
		return 0, fmt.Errorf("无效时长 %q", numText)
	}
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "m", "min", "mins", "minute", "minutes":
		return time.Duration(value) * time.Minute, nil
	case "h", "hr", "hrs", "hour", "hours":
		return time.Duration(value) * time.Hour, nil
	case "d", "day", "days":
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("不支持的时长单位 %q", unit)
	}
}

func parseChineseDuration(numText, unit string) (time.Duration, error) {
	value, err := parseChineseOrArabicNumber(numText)
	if err != nil {
		return 0, fmt.Errorf("无效时长 %q", numText)
	}
	switch strings.TrimSpace(unit) {
	case "分钟":
		return time.Duration(value) * time.Minute, nil
	case "小时":
		return time.Duration(value) * time.Hour, nil
	case "天":
		return time.Duration(value) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("不支持的时长单位 %q", unit)
	}
}

func parseChineseOrArabicNumber(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty number")
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed, nil
	}

	digits := map[rune]int{
		'零': 0,
		'〇': 0,
		'一': 1,
		'二': 2,
		'两': 2,
		'三': 3,
		'四': 4,
		'五': 5,
		'六': 6,
		'七': 7,
		'八': 8,
		'九': 9,
	}

	total := 0
	current := 0
	for _, r := range value {
		switch r {
		case '零', '〇':
			continue
		case '十':
			if current == 0 {
				current = 1
			}
			total += current * 10
			current = 0
		case '百':
			if current == 0 {
				current = 1
			}
			total += current * 100
			current = 0
		default:
			digit, ok := digits[r]
			if !ok {
				return 0, fmt.Errorf("invalid chinese numeral %q", string(r))
			}
			current = digit
		}
	}

	total += current
	if total <= 0 {
		return 0, fmt.Errorf("invalid chinese number %q", value)
	}
	return total, nil
}

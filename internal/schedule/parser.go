package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/robfig/cron/v3"
)

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow,
)

func Parse(raw string) (domain.Schedule, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return domain.Schedule{}, fmt.Errorf("parse schedule: empty schedule expression")
	}

	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return domain.Schedule{}, fmt.Errorf("parse schedule: empty schedule expression")
	}

	keyword := strings.ToLower(fields[0])

	switch keyword {
	case "every":
		return parseDurationSchedule(domain.ScheduleKindInterval, keyword, fields)
	case "after":
		return parseDurationSchedule(domain.ScheduleKindOnce, keyword, fields)
	case "cron":
		return parseCronSchedule(fields)
	default:
		return domain.Schedule{}, fmt.Errorf("parse schedule %q: unsupported schedule keyword %q", raw, fields[0])
	}
}

func parseDurationSchedule(kind domain.ScheduleKind, keyword string, fields []string) (domain.Schedule, error) {
	if len(fields) != 2 {
		return domain.Schedule{}, fmt.Errorf("parse schedule %q: %s schedules require exactly one duration", strings.Join(fields, " "), keyword)
	}

	durationText := fields[1]
	duration, err := time.ParseDuration(durationText)
	if err != nil {
		return domain.Schedule{}, fmt.Errorf("parse schedule %q: parse duration: %w", strings.Join(fields, " "), err)
	}
	if duration <= 0 {
		return domain.Schedule{}, fmt.Errorf("parse schedule %q: duration must be greater than zero", strings.Join(fields, " "))
	}

	return domain.Schedule{
		Kind: kind,
		Expr: keyword + " " + durationText,
	}, nil
}

func parseCronSchedule(fields []string) (domain.Schedule, error) {
	if len(fields) != 6 {
		return domain.Schedule{}, fmt.Errorf("parse schedule %q: cron schedules require exactly five fields", strings.Join(fields, " "))
	}

	expr := strings.Join(fields[1:], " ")
	if _, err := cronParser.Parse(expr); err != nil {
		return domain.Schedule{}, fmt.Errorf("parse schedule %q: parse cron expression: %w", strings.Join(fields, " "), err)
	}

	return domain.Schedule{
		Kind: domain.ScheduleKindCron,
		Expr: "cron " + expr,
	}, nil
}

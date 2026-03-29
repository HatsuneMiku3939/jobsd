package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func Next(spec domain.Schedule, ref time.Time, loc *time.Location) (time.Time, error) {
	parsed, err := Parse(spec.Expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("calculate next run for %q: %w", spec.Expr, err)
	}
	if parsed.Kind != spec.Kind {
		return time.Time{}, fmt.Errorf(
			"calculate next run for %q: schedule kind mismatch: got %q, parsed %q",
			spec.Expr,
			spec.Kind,
			parsed.Kind,
		)
	}

	switch parsed.Kind {
	case domain.ScheduleKindInterval, domain.ScheduleKindOnce:
		durationText := strings.Fields(parsed.Expr)[1]
		duration, err := time.ParseDuration(durationText)
		if err != nil {
			return time.Time{}, fmt.Errorf("calculate next run for %q: parse duration: %w", parsed.Expr, err)
		}
		return ref.UTC().Add(duration), nil
	case domain.ScheduleKindCron:
		if loc == nil {
			loc = time.Local
		}

		expr := strings.TrimPrefix(parsed.Expr, "cron ")
		schedule, err := cronParser.Parse(expr)
		if err != nil {
			return time.Time{}, fmt.Errorf("calculate next run for %q: parse cron expression: %w", parsed.Expr, err)
		}

		return schedule.Next(ref.In(loc)).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("calculate next run for %q: unsupported schedule kind %q", parsed.Expr, parsed.Kind)
	}
}

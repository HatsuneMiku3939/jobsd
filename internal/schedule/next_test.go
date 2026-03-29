package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestNextValidSchedules(t *testing.T) {
	seoul := time.FixedZone("KST", 9*60*60)

	tests := []struct {
		name string
		spec domain.Schedule
		ref  time.Time
		loc  *time.Location
		want time.Time
	}{
		{
			name: "interval in utc",
			spec: domain.Schedule{Kind: domain.ScheduleKindInterval, Expr: "every 10m"},
			ref:  time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC),
			want: time.Date(2025, 4, 10, 10, 10, 0, 0, time.UTC),
		},
		{
			name: "once in utc",
			spec: domain.Schedule{Kind: domain.ScheduleKindOnce, Expr: "after 45s"},
			ref:  time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC),
			want: time.Date(2025, 4, 10, 10, 0, 45, 0, time.UTC),
		},
		{
			name: "cron in utc",
			spec: domain.Schedule{Kind: domain.ScheduleKindCron, Expr: "cron */5 * * * *"},
			ref:  time.Date(2025, 4, 10, 10, 2, 30, 0, time.UTC),
			loc:  time.UTC,
			want: time.Date(2025, 4, 10, 10, 5, 0, 0, time.UTC),
		},
		{
			name: "cron with location returns utc",
			spec: domain.Schedule{Kind: domain.ScheduleKindCron, Expr: "cron 0 9 * * *"},
			ref:  time.Date(2025, 4, 10, 8, 30, 0, 0, time.UTC),
			loc:  seoul,
			want: time.Date(2025, 4, 11, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "cron exact boundary moves to next occurrence",
			spec: domain.Schedule{Kind: domain.ScheduleKindCron, Expr: "cron */5 * * * *"},
			ref:  time.Date(2025, 4, 10, 10, 5, 0, 0, time.UTC),
			loc:  time.UTC,
			want: time.Date(2025, 4, 10, 10, 10, 0, 0, time.UTC),
		},
		{
			name: "round trip normalized expression",
			spec: domain.Schedule{Kind: domain.ScheduleKindInterval, Expr: "every 1h30m"},
			ref:  time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC),
			want: time.Date(2025, 4, 10, 11, 30, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Next(tt.spec, tt.ref, tt.loc)
			if err != nil {
				t.Fatalf("Next() error = %v", err)
			}
			if !got.Equal(tt.want) {
				t.Fatalf("Next() = %s, want %s", got.Format(time.RFC3339), tt.want.Format(time.RFC3339))
			}
			if got.Location() != time.UTC {
				t.Fatalf("Next() location = %v, want UTC", got.Location())
			}
		})
	}
}

func TestNextInvalidSchedules(t *testing.T) {
	tests := []struct {
		name        string
		spec        domain.Schedule
		wantErrPart string
	}{
		{
			name:        "kind mismatch",
			spec:        domain.Schedule{Kind: domain.ScheduleKindCron, Expr: "every 10m"},
			wantErrPart: "schedule kind mismatch",
		},
		{
			name:        "invalid expression",
			spec:        domain.Schedule{Kind: domain.ScheduleKindInterval, Expr: "every 0s"},
			wantErrPart: "greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Next(tt.spec, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC), time.UTC)
			if err == nil {
				t.Fatal("Next() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Fatalf("Next() error = %q, want substring %q", err.Error(), tt.wantErrPart)
			}
		})
	}
}

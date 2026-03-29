package schedule

import (
	"strings"
	"testing"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestParseValidSchedules(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  domain.Schedule
	}{
		{
			name:  "interval",
			input: "every 10m",
			want:  domain.Schedule{Kind: domain.ScheduleKindInterval, Expr: "every 10m"},
		},
		{
			name:  "interval trimmed",
			input: "  every   1h30m  ",
			want:  domain.Schedule{Kind: domain.ScheduleKindInterval, Expr: "every 1h30m"},
		},
		{
			name:  "cron normalized keyword",
			input: "CRON */5 * * * *",
			want:  domain.Schedule{Kind: domain.ScheduleKindCron, Expr: "cron */5 * * * *"},
		},
		{
			name:  "once",
			input: "after 45s",
			want:  domain.Schedule{Kind: domain.ScheduleKindOnce, Expr: "after 45s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseInvalidSchedules(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErrPart string
	}{
		{name: "empty", input: "", wantErrPart: "empty schedule expression"},
		{name: "unknown keyword", input: "sometimes 10m", wantErrPart: "unsupported schedule keyword"},
		{name: "missing duration", input: "every", wantErrPart: "require exactly one duration"},
		{name: "zero duration", input: "every 0s", wantErrPart: "greater than zero"},
		{name: "negative duration", input: "after -1m", wantErrPart: "greater than zero"},
		{name: "extra duration token", input: "after 10m now", wantErrPart: "require exactly one duration"},
		{name: "cron missing field", input: "cron * * * *", wantErrPart: "require exactly five fields"},
		{name: "cron extra field", input: "cron * * * * * *", wantErrPart: "require exactly five fields"},
		{name: "cron invalid field", input: "cron nope * * * *", wantErrPart: "parse cron expression"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.input)
			if err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Fatalf("Parse() error = %q, want substring %q", err.Error(), tt.wantErrPart)
			}
		})
	}
}

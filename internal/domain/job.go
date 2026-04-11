package domain

import "time"

type Job struct {
	ID                       int64
	Name                     string
	Command                  string
	ScheduleKind             ScheduleKind
	ScheduleExpr             string
	Timezone                 string
	Enabled                  bool
	ConcurrencyPolicy        ConcurrencyPolicy
	OnFinish                 *OnFinishConfig
	DisableInheritedOnFinish bool
	NextRunAt                *time.Time
	LastRunAt                *time.Time
	LastRunStatus            *RunStatus
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

package domain

import "time"

type ScheduleKind string

const (
	ScheduleKindInterval ScheduleKind = "interval"
	ScheduleKindCron     ScheduleKind = "cron"
	ScheduleKindOnce     ScheduleKind = "once"
)

func (k ScheduleKind) IsValid() bool {
	switch k {
	case ScheduleKindInterval, ScheduleKindCron, ScheduleKindOnce:
		return true
	default:
		return false
	}
}

type RunTriggerType string

const (
	RunTriggerTypeSchedule RunTriggerType = "schedule"
	RunTriggerTypeManual   RunTriggerType = "manual"
)

func (t RunTriggerType) IsValid() bool {
	switch t {
	case RunTriggerTypeSchedule, RunTriggerTypeManual:
		return true
	default:
		return false
	}
}

type RunStatus string

const (
	RunStatusPending   RunStatus = "pending"
	RunStatusRunning   RunStatus = "running"
	RunStatusSucceeded RunStatus = "succeeded"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCanceled  RunStatus = "canceled"
)

func (s RunStatus) IsValid() bool {
	switch s {
	case RunStatusPending, RunStatusRunning, RunStatusSucceeded, RunStatusFailed, RunStatusCanceled:
		return true
	default:
		return false
	}
}

type ConcurrencyPolicy string

const (
	ConcurrencyPolicyForbid  ConcurrencyPolicy = "forbid"
	ConcurrencyPolicyQueue   ConcurrencyPolicy = "queue"
	ConcurrencyPolicyReplace ConcurrencyPolicy = "replace"
)

func (p ConcurrencyPolicy) IsValid() bool {
	switch p {
	case ConcurrencyPolicyForbid, ConcurrencyPolicyQueue, ConcurrencyPolicyReplace:
		return true
	default:
		return false
	}
}

type RunOutput struct {
	Stdout          string
	Stderr          string
	StdoutTruncated bool
	StderrTruncated bool
	UpdatedAt       time.Time
}

type InstanceMetadata struct {
	InstanceName  string
	CreatedAt     time.Time
	SchedulerPort int
}

type SchedulerState struct {
	Instance  string
	PID       int
	Port      int
	Token     string
	DBPath    string
	StartedAt time.Time
	Version   string
}

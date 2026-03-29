package domain

import "time"

type Run struct {
	ID           int64
	JobID        int64
	JobName      string
	TriggerType  RunTriggerType
	Status       RunStatus
	ScheduledFor *time.Time
	QueuedAt     time.Time
	StartedAt    *time.Time
	FinishedAt   *time.Time
	ExitCode     *int
	ErrorMessage *string
	RunnerID     *string
	Output       *RunOutput
}

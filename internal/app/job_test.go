package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestJobCommandsRequireFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "add missing instance",
			args: []string{"job", "add", "--name", "cleanup", "--schedule", "every 10m", "--command", "echo cleanup"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "add missing name",
			args: []string{"job", "add", "--instance", "dev", "--schedule", "every 10m", "--command", "echo cleanup"},
			want: `required flag(s) "name" not set`,
		},
		{
			name: "get missing name",
			args: []string{"job", "get", "--instance", "dev"},
			want: `required flag(s) "name" not set`,
		},
		{
			name: "update missing instance",
			args: []string{"job", "update", "--name", "cleanup", "--command", "echo updated"},
			want: `required flag(s) "instance" not set`,
		},
		{
			name: "delete missing name",
			args: []string{"job", "delete", "--instance", "dev"},
			want: `required flag(s) "name" not set`,
		},
		{
			name: "pause missing name",
			args: []string{"job", "pause", "--instance", "dev"},
			want: `required flag(s) "name" not set`,
		},
		{
			name: "resume missing name",
			args: []string{"job", "resume", "--instance", "dev"},
			want: `required flag(s) "name" not set`,
		},
		{
			name: "run missing name",
			args: []string{"job", "run", "--instance", "dev"},
			want: `required flag(s) "name" not set`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestDirs(t)

			_, err := executeRootCommand(t, tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if err.Error() != tt.want {
				t.Fatalf("Execute() error = %q, want %q", err.Error(), tt.want)
			}
		})
	}
}

func TestJobCommandValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "invalid schedule",
			args: []string{"job", "add", "--instance", "dev", "--name", "cleanup", "--schedule", "bad", "--command", "echo cleanup"},
			want: `unsupported schedule keyword`,
		},
		{
			name: "invalid timezone",
			args: []string{"job", "add", "--instance", "dev", "--name", "cleanup", "--schedule", "every 10m", "--command", "echo cleanup", "--timezone", "Mars/Phobos"},
			want: `load timezone`,
		},
		{
			name: "invalid policy",
			args: []string{"job", "add", "--instance", "dev", "--name", "cleanup", "--schedule", "every 10m", "--command", "echo cleanup", "--concurrency-policy", "parallel"},
			want: `invalid concurrency policy`,
		},
		{
			name: "update without fields",
			args: []string{"job", "update", "--instance", "dev", "--name", "cleanup"},
			want: `at least one field to update must be provided`,
		},
		{
			name: "update conflicting enable disable",
			args: []string{"job", "update", "--instance", "dev", "--name", "cleanup", "--enabled", "--disabled"},
			want: `--enabled and --disabled cannot be used together`,
		},
		{
			name: "add conflicting on finish and disable inherited",
			args: []string{"job", "add", "--instance", "dev", "--name", "cleanup", "--schedule", "every 10m", "--command", "echo cleanup", "--on-finish-config-json", `{"type":"command","command":{"program":"echo"}}`, "--disable-inherited-on-finish"},
			want: `--on-finish-config-json and --disable-inherited-on-finish cannot be used together`,
		},
		{
			name: "update conflicting on finish and clear",
			args: []string{"job", "update", "--instance", "dev", "--name", "cleanup", "--on-finish-config-json", `{"type":"command","command":{"program":"echo"}}`, "--clear-on-finish"},
			want: `--on-finish-config-json and --clear-on-finish cannot be used together`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTestDirs(t)

			_, err := executeRootCommand(t, tt.args...)
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Execute() error = %q, want substring %q", err.Error(), tt.want)
			}
		})
	}
}

func TestJobAddListGetDeleteIntegration(t *testing.T) {
	setTestDirs(t)
	setFixedCurrentTime(t, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))

	stdout, err := executeRootCommand(
		t,
		"--output", "json",
		"job", "add",
		"--instance", "dev",
		"--name", "cleanup",
		"--schedule", "every 10m",
		"--command", "echo cleanup",
		"--timezone", "UTC",
		"--concurrency-policy", "queue",
	)
	if err != nil {
		t.Fatalf("job add error = %v", err)
	}

	var added jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &added); err != nil {
		t.Fatalf("Unmarshal(add) error = %v", err)
	}
	if added.Name != "cleanup" {
		t.Fatalf("added.Name = %q, want cleanup", added.Name)
	}
	if added.NextRunAt == nil || *added.NextRunAt != "2025-04-10T10:10:00Z" {
		t.Fatalf("added.NextRunAt = %v, want 2025-04-10T10:10:00Z", added.NextRunAt)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "job", "list", "--instance", "dev")
	if err != nil {
		t.Fatalf("job list error = %v", err)
	}

	var listed []jobSummaryOutput
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("Unmarshal(list) error = %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "cleanup" {
		t.Fatalf("listed = %#v, want one cleanup job", listed)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "job", "get", "--instance", "dev", "--name", "cleanup")
	if err != nil {
		t.Fatalf("job get error = %v", err)
	}

	var got jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("Unmarshal(get) error = %v", err)
	}
	if got.Command != "echo cleanup" {
		t.Fatalf("got.Command = %q, want echo cleanup", got.Command)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "job", "delete", "--instance", "dev", "--name", "cleanup")
	if err != nil {
		t.Fatalf("job delete error = %v", err)
	}

	var deleted deleteResultOutput
	if err := json.Unmarshal([]byte(stdout), &deleted); err != nil {
		t.Fatalf("Unmarshal(delete) error = %v", err)
	}
	if !deleted.Deleted {
		t.Fatalf("deleted = %#v, want true", deleted)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "job", "list", "--instance", "dev")
	if err != nil {
		t.Fatalf("job list after delete error = %v", err)
	}

	listed = nil
	if err := json.Unmarshal([]byte(stdout), &listed); err != nil {
		t.Fatalf("Unmarshal(list after delete) error = %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("len(listed after delete) = %d, want 0", len(listed))
	}
}

func TestJobAddDisabledAndOneTimeIntegration(t *testing.T) {
	setTestDirs(t)
	setFixedCurrentTime(t, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))

	stdout, err := executeRootCommand(
		t,
		"--output", "json",
		"job", "add",
		"--instance", "dev",
		"--name", "disabled",
		"--schedule", "every 15m",
		"--command", "echo disabled",
		"--disabled",
	)
	if err != nil {
		t.Fatalf("disabled add error = %v", err)
	}

	var disabledJob jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &disabledJob); err != nil {
		t.Fatalf("Unmarshal(disabled add) error = %v", err)
	}
	if disabledJob.Enabled {
		t.Fatalf("disabledJob.Enabled = true, want false")
	}
	if disabledJob.NextRunAt != nil {
		t.Fatalf("disabledJob.NextRunAt = %v, want nil", disabledJob.NextRunAt)
	}

	stdout, err = executeRootCommand(
		t,
		"--output", "json",
		"job", "add",
		"--instance", "dev",
		"--name", "once",
		"--schedule", "after 10m",
		"--command", "echo once",
		"--timezone", "UTC",
	)
	if err != nil {
		t.Fatalf("one-time add error = %v", err)
	}

	var onceJob jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &onceJob); err != nil {
		t.Fatalf("Unmarshal(one-time add) error = %v", err)
	}
	if onceJob.NextRunAt == nil || *onceJob.NextRunAt != "2025-04-10T10:10:00Z" {
		t.Fatalf("onceJob.NextRunAt = %v, want 2025-04-10T10:10:00Z", onceJob.NextRunAt)
	}
}

func TestJobOnFinishIntegration(t *testing.T) {
	setTestDirs(t)
	setFixedCurrentTime(t, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))

	stdout, err := executeRootCommand(
		t,
		"--output", "json",
		"job", "add",
		"--instance", "dev",
		"--name", "cleanup",
		"--schedule", "every 10m",
		"--command", "echo cleanup",
		"--on-finish-config-json", `{"type":"command","command":{"program":"echo","args":["hook"]}}`,
	)
	if err != nil {
		t.Fatalf("job add error = %v", err)
	}

	var added jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &added); err != nil {
		t.Fatalf("Unmarshal(add) error = %v", err)
	}
	if added.OnFinish == nil || added.OnFinish.Command == nil || added.OnFinish.Command.Program != "echo" {
		t.Fatalf("added.OnFinish = %#v, want command config", added.OnFinish)
	}
	if added.DisableInheritedOnFinish {
		t.Fatal("added.DisableInheritedOnFinish = true, want false")
	}

	stdout, err = executeRootCommand(
		t,
		"--output", "json",
		"job", "update",
		"--instance", "dev",
		"--name", "cleanup",
		"--disable-inherited-on-finish",
	)
	if err != nil {
		t.Fatalf("job update error = %v", err)
	}

	var updated jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &updated); err != nil {
		t.Fatalf("Unmarshal(update) error = %v", err)
	}
	if updated.OnFinish != nil {
		t.Fatalf("updated.OnFinish = %#v, want nil", updated.OnFinish)
	}
	if !updated.DisableInheritedOnFinish {
		t.Fatal("updated.DisableInheritedOnFinish = false, want true")
	}
}

func TestJobHelpIncludesJobsdExamples(t *testing.T) {
	setTestDirs(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "job command",
			args: []string{"job", "--help"},
			want: "jobsd job list --instance dev",
		},
		{
			name: "job add",
			args: []string{"job", "add", "--help"},
			want: "jobsd job add \\",
		},
		{
			name: "job update",
			args: []string{"job", "update", "--help"},
			want: "jobsd job update \\",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, err := executeRootCommand(t, tt.args...)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !strings.Contains(stdout, tt.want) {
				t.Fatalf("stdout = %q, want substring %q", stdout, tt.want)
			}
		})
	}
}

func TestJobTableOutputIsStable(t *testing.T) {
	setTestDirs(t)
	setFixedCurrentTime(t, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))

	if _, err := executeRootCommand(
		t,
		"job", "add",
		"--instance", "dev",
		"--name", "cleanup",
		"--schedule", "every 10m",
		"--command", "echo cleanup",
		"--timezone", "UTC",
	); err != nil {
		t.Fatalf("job add error = %v", err)
	}

	stdout, err := executeRootCommand(t, "job", "get", "--instance", "dev", "--name", "cleanup")
	if err != nil {
		t.Fatalf("job get error = %v", err)
	}

	const want = "" +
		"FIELD                        VALUE\n" +
		"ID                           1\n" +
		"NAME                         cleanup\n" +
		"COMMAND                      echo cleanup\n" +
		"SCHEDULE                     every 10m\n" +
		"TIMEZONE                     UTC\n" +
		"ENABLED                      true\n" +
		"CONCURRENCY_POLICY           forbid\n" +
		"ON_FINISH                    \n" +
		"DISABLE_INHERITED_ON_FINISH  false\n" +
		"NEXT_RUN_AT                  2025-04-10T10:10:00Z\n" +
		"LAST_RUN_AT                  \n" +
		"LAST_RUN_STATUS              \n" +
		"CREATED_AT                   2025-04-10T10:00:00Z\n" +
		"UPDATED_AT                   2025-04-10T10:00:00Z\n"
	if stdout != want {
		t.Fatalf("job get table output = %q, want %q", stdout, want)
	}
}

func TestJobUpdatePauseResumeAndRunIntegration(t *testing.T) {
	setTestDirs(t)
	setFixedCurrentTime(t, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))

	if _, err := executeRootCommand(
		t,
		"job", "add",
		"--instance", "dev",
		"--name", "cleanup",
		"--schedule", "after 10m",
		"--command", "echo cleanup",
		"--timezone", "UTC",
	); err != nil {
		t.Fatalf("job add error = %v", err)
	}

	setFixedCurrentTime(t, time.Date(2025, 4, 10, 10, 30, 0, 0, time.UTC))
	stdout, err := executeRootCommand(
		t,
		"--output", "json",
		"job", "update",
		"--instance", "dev",
		"--name", "cleanup",
		"--new-name", "cleanup-nightly",
		"--command", "echo nightly",
		"--schedule", "after 20m",
		"--timezone", "Asia/Seoul",
		"--concurrency-policy", "forbid",
	)
	if err != nil {
		t.Fatalf("job update error = %v", err)
	}

	var updated jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &updated); err != nil {
		t.Fatalf("Unmarshal(update) error = %v", err)
	}
	if updated.Name != "cleanup-nightly" {
		t.Fatalf("updated.Name = %q, want cleanup-nightly", updated.Name)
	}
	if updated.Command != "echo nightly" {
		t.Fatalf("updated.Command = %q, want echo nightly", updated.Command)
	}
	if updated.NextRunAt == nil || *updated.NextRunAt != "2025-04-10T10:50:00Z" {
		t.Fatalf("updated.NextRunAt = %v, want 2025-04-10T10:50:00Z", updated.NextRunAt)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "job", "pause", "--instance", "dev", "--name", "cleanup-nightly")
	if err != nil {
		t.Fatalf("job pause error = %v", err)
	}

	var paused jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &paused); err != nil {
		t.Fatalf("Unmarshal(pause) error = %v", err)
	}
	if paused.Enabled {
		t.Fatalf("paused.Enabled = true, want false")
	}
	if paused.NextRunAt != nil {
		t.Fatalf("paused.NextRunAt = %v, want nil", paused.NextRunAt)
	}

	setFixedCurrentTime(t, time.Date(2025, 4, 10, 11, 0, 0, 0, time.UTC))
	stdout, err = executeRootCommand(t, "--output", "json", "job", "resume", "--instance", "dev", "--name", "cleanup-nightly")
	if err != nil {
		t.Fatalf("job resume error = %v", err)
	}

	var resumed jobDetailOutput
	if err := json.Unmarshal([]byte(stdout), &resumed); err != nil {
		t.Fatalf("Unmarshal(resume) error = %v", err)
	}
	if !resumed.Enabled {
		t.Fatalf("resumed.Enabled = false, want true")
	}
	if resumed.NextRunAt == nil || *resumed.NextRunAt != "2025-04-10T11:20:00Z" {
		t.Fatalf("resumed.NextRunAt = %v, want 2025-04-10T11:20:00Z", resumed.NextRunAt)
	}

	stdout, err = executeRootCommand(t, "--output", "json", "job", "run", "--instance", "dev", "--name", "cleanup-nightly")
	if err != nil {
		t.Fatalf("job run error = %v", err)
	}

	var enqueued runEnqueueOutput
	if err := json.Unmarshal([]byte(stdout), &enqueued); err != nil {
		t.Fatalf("Unmarshal(job run) error = %v", err)
	}
	if enqueued.RunID == 0 {
		t.Fatal("enqueued.RunID = 0, want non-zero")
	}
	if enqueued.Status != string(domain.RunStatusPending) {
		t.Fatalf("enqueued.Status = %q, want pending", enqueued.Status)
	}

	db, cleanup := openInstanceDBForTest(t, "dev")
	defer cleanup()

	run, err := db.Runs.Get(context.Background(), enqueued.RunID)
	if err != nil {
		t.Fatalf("RunStore.Get() error = %v", err)
	}
	if run.TriggerType != domain.RunTriggerTypeManual {
		t.Fatalf("run.TriggerType = %q, want manual", run.TriggerType)
	}

	_, err = executeRootCommand(t, "job", "run", "--instance", "dev", "--name", "cleanup-nightly")
	if err == nil {
		t.Fatal("second job run error = nil, want conflict")
	}
	if !strings.Contains(err.Error(), `already has a pending or running run`) {
		t.Fatalf("second job run error = %q, want conflict message", err.Error())
	}
}

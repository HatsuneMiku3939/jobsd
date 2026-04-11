package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestOnFinishDispatcherCommandUsesJobOverride(t *testing.T) {
	var (
		gotProgram string
		gotArgs    []string
		gotBody    []byte
		gotEnv     []string
	)
	recorder := &fakeHookDeliveryRecorder{}
	dispatcher := &OnFinishDispatcher{
		CommandRunner: func(ctx context.Context, program string, args []string, payload []byte, env []string) error {
			gotProgram = program
			gotArgs = append([]string(nil), args...)
			gotBody = append([]byte(nil), payload...)
			gotEnv = append([]string(nil), env...)
			return nil
		},
		DeliveryRecorder: recorder,
		Now: func() time.Time {
			return time.Date(2025, 4, 10, 10, 2, 0, 0, time.UTC)
		},
	}

	job := domain.Job{
		ID:           7,
		Name:         "cleanup",
		Command:      "echo cleanup",
		ScheduleExpr: "every 10m",
		OnFinish: &domain.OnFinishConfig{
			Type: domain.OnFinishSinkTypeCommand,
			Command: &domain.CommandSinkConfig{
				Program: "hook-handler",
				Args:    []string{"--from", "jobsd"},
			},
		},
	}
	startedAt := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	finishedAt := time.Date(2025, 4, 10, 10, 0, 3, 0, time.UTC)
	run := domain.Run{
		ID:         11,
		JobID:      job.ID,
		JobName:    job.Name,
		Status:     domain.RunStatusSucceeded,
		StartedAt:  &startedAt,
		FinishedAt: &finishedAt,
		ExitCode:   intPointer(0),
		Output: &domain.RunOutput{
			Stdout: "hello",
			Stderr: "warn",
		},
	}

	if err := dispatcher.Notify(context.Background(), "dev", job, run); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if gotProgram != "hook-handler" {
		t.Fatalf("program = %q, want hook-handler", gotProgram)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "--from" || gotArgs[1] != "jobsd" {
		t.Fatalf("args = %#v, want hook args", gotArgs)
	}

	var payload runFinishedPayload
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if payload.Event != "run.finished" || payload.Instance != "dev" || payload.JobName != "cleanup" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.DurationMS != 3000 {
		t.Fatalf("DurationMS = %d, want 3000", payload.DurationMS)
	}
	if payload.StdoutPreview != "hello" || payload.StderrPreview != "warn" {
		t.Fatalf("previews = stdout:%q stderr:%q", payload.StdoutPreview, payload.StderrPreview)
	}

	if len(gotEnv) != 3 {
		t.Fatalf("env length = %d, want 3", len(gotEnv))
	}
	if len(recorder.deliveries) != 1 || recorder.deliveries[0].Status != domain.HookDeliveryStatusSucceeded {
		t.Fatalf("deliveries = %#v, want one succeeded delivery", recorder.deliveries)
	}
}

func TestOnFinishDispatcherHTTPRetriesOnServerError(t *testing.T) {
	recorder := &fakeHookDeliveryRecorder{}
	attempts := 0
	dispatcher := &OnFinishDispatcher{
		MetadataReader: &fakeHookMetadataReader{
			meta: domain.InstanceMetadata{
				InstanceName: "dev",
				OnFinish: &domain.OnFinishConfig{
					Type: domain.OnFinishSinkTypeHTTP,
					HTTP: &domain.HTTPSinkConfig{
						URL: "http://127.0.0.1:8080/hooks",
					},
				},
			},
		},
		DeliveryRecorder: recorder,
		HTTPDoer: func(req *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return &http.Response{StatusCode: 500, Body: http.NoBody}, nil
			}
			return &http.Response{StatusCode: 204, Body: http.NoBody}, nil
		},
		Now: func() time.Time {
			return time.Date(2025, 4, 10, 10, 2, 0, 0, time.UTC)
		},
		Sleep: func(time.Duration) {},
	}

	job := domain.Job{
		ID:           7,
		Name:         "cleanup",
		Command:      "echo cleanup",
		ScheduleExpr: "every 10m",
	}
	run := domain.Run{
		ID:      11,
		JobID:   job.ID,
		JobName: job.Name,
		Status:  domain.RunStatusFailed,
	}

	if err := dispatcher.Notify(context.Background(), "dev", job, run); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(recorder.deliveries) != 2 {
		t.Fatalf("deliveries length = %d, want 2", len(recorder.deliveries))
	}
	if recorder.deliveries[0].Status != domain.HookDeliveryStatusFailed || recorder.deliveries[1].Status != domain.HookDeliveryStatusSucceeded {
		t.Fatalf("deliveries = %#v", recorder.deliveries)
	}
}

func TestOnFinishDispatcherSkipsDisabledInheritance(t *testing.T) {
	dispatcher := &OnFinishDispatcher{
		MetadataReader: &fakeHookMetadataReader{
			meta: domain.InstanceMetadata{
				InstanceName: "dev",
				OnFinish: &domain.OnFinishConfig{
					Type: domain.OnFinishSinkTypeCommand,
					Command: &domain.CommandSinkConfig{
						Program: "echo",
					},
				},
			},
		},
		CommandRunner: func(ctx context.Context, program string, args []string, payload []byte, env []string) error {
			t.Fatal("CommandRunner() should not be called")
			return nil
		},
	}

	job := domain.Job{
		Name:                     "cleanup",
		DisableInheritedOnFinish: true,
	}
	run := domain.Run{ID: 1}

	if err := dispatcher.Notify(context.Background(), "dev", job, run); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
}

func TestLoopHookFailureDoesNotChangeRunResult(t *testing.T) {
	now := time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC)
	job := loopTestJob(1, "manual", domain.ConcurrencyPolicyQueue, "every 10m", domain.ScheduleKindInterval)
	jobStore := newFakeLoopJobStore(job)
	runStore := newFakeLoopRunStore()
	runStore.addRun(domain.Run{
		ID:          1,
		JobID:       job.ID,
		JobName:     job.Name,
		TriggerType: domain.RunTriggerTypeManual,
		Status:      domain.RunStatusPending,
		QueuedAt:    now.Add(-1 * time.Minute),
	})

	executor := &fakeExecutor{
		execute: func(ctx context.Context, command string) ExecutionResult {
			return ExecutionResult{
				Status:     domain.RunStatusSucceeded,
				StartedAt:  now,
				FinishedAt: now.Add(2 * time.Second),
				ExitCode:   intPointer(0),
			}
		},
	}

	tick := make(chan time.Time, 1)
	loopCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- (&Loop{
			Instance: "dev",
			JobStore: jobStore,
			RunStore: runStore,
			Executor: executor,
			OnFinishNotifier: onFinishNotifierFunc(func(ctx context.Context, instance string, job domain.Job, run domain.Run) error {
				return errors.New("hook failed")
			}),
			Tick: tick,
			Now:  func() time.Time { return now },
		}).Run(loopCtx)
	}()

	tick <- now
	waitForLoop(t, errCh, func() bool {
		run, _ := runStore.Get(context.Background(), 1)
		return run.Status == domain.RunStatusSucceeded
	})
}

type fakeHookMetadataReader struct {
	meta domain.InstanceMetadata
	err  error
}

func (r *fakeHookMetadataReader) Get(ctx context.Context) (domain.InstanceMetadata, error) {
	return r.meta, r.err
}

type fakeHookDeliveryRecorder struct {
	deliveries []domain.RunHookDelivery
}

func (r *fakeHookDeliveryRecorder) Create(ctx context.Context, delivery domain.RunHookDelivery) (domain.RunHookDelivery, error) {
	delivery.ID = int64(len(r.deliveries) + 1)
	r.deliveries = append(r.deliveries, delivery)
	return delivery, nil
}

type onFinishNotifierFunc func(ctx context.Context, instance string, job domain.Job, run domain.Run) error

func (f onFinishNotifierFunc) Notify(ctx context.Context, instance string, job domain.Job, run domain.Run) error {
	return f(ctx, instance, job, run)
}

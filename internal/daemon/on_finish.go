package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
	"github.com/hatsunemiku3939/jobsd/internal/sqlite"
)

const runFinishedEvent = "run.finished"

type OnFinishNotifier interface {
	Notify(ctx context.Context, instance string, job domain.Job, run domain.Run) error
}

type onFinishMetadataReader interface {
	Get(ctx context.Context) (domain.InstanceMetadata, error)
}

type onFinishDeliveryRecorder interface {
	Create(ctx context.Context, delivery domain.RunHookDelivery) (domain.RunHookDelivery, error)
}

type commandHookRunner func(ctx context.Context, program string, args []string, payload []byte, env []string) error
type httpHookDoer func(req *http.Request) (*http.Response, error)

type OnFinishDispatcher struct {
	MetadataReader   onFinishMetadataReader
	DeliveryRecorder onFinishDeliveryRecorder
	Logger           *slog.Logger
	Now              func() time.Time
	Sleep            func(time.Duration)
	CommandRunner    commandHookRunner
	HTTPDoer         httpHookDoer
}

type runFinishedPayload struct {
	Version       int     `json:"version"`
	Event         string  `json:"event"`
	Instance      string  `json:"instance"`
	JobName       string  `json:"job_name"`
	RunID         int64   `json:"run_id"`
	Schedule      string  `json:"schedule"`
	Command       string  `json:"command"`
	Status        string  `json:"status"`
	ExitCode      *int    `json:"exit_code"`
	StartedAt     string  `json:"started_at"`
	FinishedAt    string  `json:"finished_at"`
	DurationMS    int64   `json:"duration_ms"`
	StdoutPreview string  `json:"stdout_preview"`
	StderrPreview string  `json:"stderr_preview"`
	StdoutPath    *string `json:"stdout_path"`
	StderrPath    *string `json:"stderr_path"`
}

type hookAttemptResult struct {
	Status         domain.HookDeliveryStatus
	HTTPStatusCode *int
	ErrorMessage   *string
	Retryable      bool
}

func (d *OnFinishDispatcher) Notify(ctx context.Context, instance string, job domain.Job, run domain.Run) error {
	config, err := d.resolveConfig(ctx, job)
	if err != nil {
		return err
	}
	if config == nil {
		return nil
	}

	payload := buildRunFinishedPayload(instance, job, run)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal on_finish payload: %w", err)
	}

	attempts := config.RetryCount + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		startedAt := d.now().UTC()
		result := d.deliverAttempt(ctx, *config, body, payload)
		finishedAt := d.now().UTC()

		if err := d.recordAttempt(ctx, run.ID, *config, attempt, startedAt, finishedAt, result); err != nil {
			d.logger().Error(
				"record on_finish hook delivery failed",
				slog.Int64("run_id", run.ID),
				slog.Int("attempt", attempt),
				slog.String("error", err.Error()),
			)
		}

		if result.Status == domain.HookDeliveryStatusSucceeded {
			return nil
		}
		if !result.Retryable || attempt == attempts {
			return nil
		}
		d.sleep(time.Duration(config.RetryBackoffMS) * time.Millisecond)
	}

	return nil
}

func (d *OnFinishDispatcher) resolveConfig(ctx context.Context, job domain.Job) (*domain.OnFinishConfig, error) {
	if job.OnFinish != nil {
		return normalizeResolvedOnFinish(job.OnFinish)
	}
	if job.DisableInheritedOnFinish {
		return nil, nil
	}
	if d.MetadataReader == nil {
		return nil, nil
	}

	meta, err := d.MetadataReader.Get(ctx)
	switch {
	case err == nil:
		return normalizeResolvedOnFinish(meta.OnFinish)
	case err == sqlite.ErrMetadataNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("read instance metadata: %w", err)
	}
}

func normalizeResolvedOnFinish(config *domain.OnFinishConfig) (*domain.OnFinishConfig, error) {
	if config == nil {
		return nil, nil
	}

	normalized, err := domain.NormalizeOnFinishConfig(*config)
	if err != nil {
		return nil, err
	}

	return &normalized, nil
}

func (d *OnFinishDispatcher) deliverAttempt(
	ctx context.Context,
	config domain.OnFinishConfig,
	body []byte,
	payload runFinishedPayload,
) hookAttemptResult {
	switch config.Type {
	case domain.OnFinishSinkTypeCommand:
		return d.deliverCommand(ctx, config, body, payload)
	case domain.OnFinishSinkTypeHTTP:
		return d.deliverHTTP(ctx, config, body)
	default:
		message := fmt.Sprintf("unsupported on_finish type %q", config.Type)
		return hookAttemptResult{
			Status:       domain.HookDeliveryStatusFailed,
			ErrorMessage: stringPointer(message),
			Retryable:    false,
		}
	}
}

func (d *OnFinishDispatcher) deliverCommand(
	ctx context.Context,
	config domain.OnFinishConfig,
	body []byte,
	payload runFinishedPayload,
) hookAttemptResult {
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMS)*time.Millisecond)
	defer cancel()

	runner := d.CommandRunner
	if runner == nil {
		runner = runCommandHook
	}

	err := runner(attemptCtx, config.Command.Program, config.Command.Args, body, []string{
		"JOBSD_EVENT=" + payload.Event,
		"JOBSD_INSTANCE=" + payload.Instance,
		"JOBSD_RUN_ID=" + fmt.Sprintf("%d", payload.RunID),
	})
	if err == nil {
		return hookAttemptResult{Status: domain.HookDeliveryStatusSucceeded}
	}
	if attemptCtx.Err() == context.DeadlineExceeded {
		message := fmt.Sprintf("command hook timed out after %dms", config.TimeoutMS)
		return hookAttemptResult{
			Status:       domain.HookDeliveryStatusTimedOut,
			ErrorMessage: stringPointer(message),
			Retryable:    true,
		}
	}

	return hookAttemptResult{
		Status:       domain.HookDeliveryStatusFailed,
		ErrorMessage: stringPointer(err.Error()),
		Retryable:    true,
	}
}

func (d *OnFinishDispatcher) deliverHTTP(ctx context.Context, config domain.OnFinishConfig, body []byte) hookAttemptResult {
	attemptCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMS)*time.Millisecond)
	defer cancel()

	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, config.HTTP.URL, bytes.NewReader(body))
	if err != nil {
		return hookAttemptResult{
			Status:       domain.HookDeliveryStatusFailed,
			ErrorMessage: stringPointer(err.Error()),
			Retryable:    false,
		}
	}
	for key, value := range config.HTTP.Headers {
		request.Header.Set(key, value)
	}
	request.Header.Set("Content-Type", "application/json")

	doer := d.HTTPDoer
	if doer == nil {
		client := &http.Client{}
		doer = client.Do
	}

	response, err := doer(request)
	if err != nil {
		if attemptCtx.Err() == context.DeadlineExceeded {
			message := fmt.Sprintf("http hook timed out after %dms", config.TimeoutMS)
			return hookAttemptResult{
				Status:       domain.HookDeliveryStatusTimedOut,
				ErrorMessage: stringPointer(message),
				Retryable:    true,
			}
		}
		return hookAttemptResult{
			Status:       domain.HookDeliveryStatusFailed,
			ErrorMessage: stringPointer(err.Error()),
			Retryable:    true,
		}
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	statusCode := response.StatusCode
	switch {
	case statusCode >= 200 && statusCode < 300:
		return hookAttemptResult{
			Status:         domain.HookDeliveryStatusSucceeded,
			HTTPStatusCode: &statusCode,
		}
	case statusCode >= 500:
		message := fmt.Sprintf("http hook returned status %d", statusCode)
		return hookAttemptResult{
			Status:         domain.HookDeliveryStatusFailed,
			HTTPStatusCode: &statusCode,
			ErrorMessage:   stringPointer(message),
			Retryable:      true,
		}
	default:
		message := fmt.Sprintf("http hook returned status %d", statusCode)
		return hookAttemptResult{
			Status:         domain.HookDeliveryStatusFailed,
			HTTPStatusCode: &statusCode,
			ErrorMessage:   stringPointer(message),
			Retryable:      false,
		}
	}
}

func (d *OnFinishDispatcher) recordAttempt(
	ctx context.Context,
	runID int64,
	config domain.OnFinishConfig,
	attempt int,
	startedAt time.Time,
	finishedAt time.Time,
	result hookAttemptResult,
) error {
	if d.DeliveryRecorder == nil {
		return nil
	}

	_, err := d.DeliveryRecorder.Create(ctx, domain.RunHookDelivery{
		RunID:          runID,
		Event:          runFinishedEvent,
		SinkType:       config.Type,
		Attempt:        attempt,
		Status:         result.Status,
		HTTPStatusCode: result.HTTPStatusCode,
		ErrorMessage:   result.ErrorMessage,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
	})
	if err != nil {
		return fmt.Errorf("create run hook delivery: %w", err)
	}

	return nil
}

func (d *OnFinishDispatcher) logger() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func (d *OnFinishDispatcher) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now().UTC()
}

func (d *OnFinishDispatcher) sleep(duration time.Duration) {
	if d.Sleep != nil {
		d.Sleep(duration)
		return
	}
	time.Sleep(duration)
}

func buildRunFinishedPayload(instance string, job domain.Job, run domain.Run) runFinishedPayload {
	finishedAt := time.Now().UTC()
	if run.FinishedAt != nil {
		finishedAt = run.FinishedAt.UTC()
	}

	startedAt := finishedAt
	if run.StartedAt != nil {
		startedAt = run.StartedAt.UTC()
	}

	return runFinishedPayload{
		Version:       1,
		Event:         runFinishedEvent,
		Instance:      instance,
		JobName:       job.Name,
		RunID:         run.ID,
		Schedule:      job.ScheduleExpr,
		Command:       job.Command,
		Status:        string(run.Status),
		ExitCode:      cloneIntPointer(run.ExitCode),
		StartedAt:     startedAt.Format(time.RFC3339),
		FinishedAt:    finishedAt.Format(time.RFC3339),
		DurationMS:    finishedAt.Sub(startedAt).Milliseconds(),
		StdoutPreview: previewHookOutput(run.Output, true),
		StderrPreview: previewHookOutput(run.Output, false),
		StdoutPath:    nil,
		StderrPath:    nil,
	}
}

func previewHookOutput(output *domain.RunOutput, stdout bool) string {
	if output == nil {
		return ""
	}

	text := output.Stderr
	if stdout {
		text = output.Stdout
	}

	preview := []byte(text)
	if len(preview) <= domain.DefaultOnFinishPreviewMaxBytes {
		return text
	}

	return string(preview[:domain.DefaultOnFinishPreviewMaxBytes])
}

func runCommandHook(ctx context.Context, program string, args []string, payload []byte, env []string) error {
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = append(os.Environ(), env...)
	return cmd.Run()
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/config"
	"github.com/hatsunemiku3939/jobsd/internal/daemon"
	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

var (
	schedulerStartupTimeout = 5 * time.Second
	schedulerStopTimeout    = 5 * time.Second
	schedulerPollInterval   = 100 * time.Millisecond
	schedulerRequestTimeout = 500 * time.Millisecond
)

type schedulerAPIError struct {
	StatusCode int
	Message    string
}

func (e *schedulerAPIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("scheduler api returned status %d", e.StatusCode)
	}

	return fmt.Sprintf("scheduler api returned status %d: %s", e.StatusCode, e.Message)
}

type schedulerClient struct {
	httpClient *http.Client
}

type schedulerInspection struct {
	Paths         config.Paths
	State         *domain.SchedulerState
	Status        domain.SchedulerStatus
	Reason        string
	Ping          *daemon.PingResponse
	SchedulerInfo *daemon.SchedulerResponse
}

func newSchedulerClient() *schedulerClient {
	return &schedulerClient{
		httpClient: &http.Client{
			Timeout: schedulerRequestTimeout,
		},
	}
}

func inspectScheduler(ctx context.Context, instance string) (schedulerInspection, error) {
	paths, err := config.ResolvePaths(instance)
	if err != nil {
		return schedulerInspection{}, err
	}

	state, err := daemon.ReadState(paths.StatePath)
	switch {
	case errors.Is(err, daemon.ErrStateNotFound):
		return schedulerInspection{
			Paths:  paths,
			Status: domain.SchedulerStatusStopped,
		}, nil
	case err != nil:
		return schedulerInspection{
			Paths:  paths,
			Status: domain.SchedulerStatusStale,
			Reason: err.Error(),
		}, nil
	}

	inspection := schedulerInspection{
		Paths:  paths,
		State:  &state,
		Status: domain.SchedulerStatusStale,
	}
	if state.Token == "" {
		inspection.Reason = "scheduler state is missing token"
		return inspection, nil
	}

	client := newSchedulerClient()
	schedulerInfo, err := client.getScheduler(ctx, state)
	if err != nil {
		inspection.Reason = err.Error()
		return inspection, nil
	}

	ping, err := client.ping(ctx, state)
	if err != nil {
		inspection.Reason = err.Error()
		return inspection, nil
	}

	inspection.Status = domain.SchedulerStatusRunning
	inspection.Ping = &ping
	inspection.SchedulerInfo = &schedulerInfo

	return inspection, nil
}

func (c *schedulerClient) ping(ctx context.Context, state domain.SchedulerState) (daemon.PingResponse, error) {
	var response daemon.PingResponse
	if err := c.doJSON(ctx, state, http.MethodGet, "/v1/ping", http.StatusOK, &response); err != nil {
		return daemon.PingResponse{}, err
	}

	return response, nil
}

func (c *schedulerClient) getScheduler(ctx context.Context, state domain.SchedulerState) (daemon.SchedulerResponse, error) {
	var response daemon.SchedulerResponse
	if err := c.doJSON(ctx, state, http.MethodGet, "/v1/scheduler", http.StatusOK, &response); err != nil {
		return daemon.SchedulerResponse{}, err
	}

	return response, nil
}

func (c *schedulerClient) shutdown(ctx context.Context, state domain.SchedulerState) error {
	return c.doJSON(ctx, state, http.MethodPost, "/v1/scheduler/shutdown", http.StatusNoContent, nil)
}

func (c *schedulerClient) doJSON(
	ctx context.Context,
	state domain.SchedulerState,
	method string,
	path string,
	expectedStatus int,
	response any,
) error {
	req, err := http.NewRequestWithContext(ctx, method, schedulerURL(state.Port, path), nil)
	if err != nil {
		return fmt.Errorf("build scheduler request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Jobs-Token", state.Token)

	httpResponse, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("call scheduler api: %w", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != expectedStatus {
		var apiErr struct {
			Error string `json:"error"`
		}
		if err := json.NewDecoder(httpResponse.Body).Decode(&apiErr); err != nil {
			return &schedulerAPIError{StatusCode: httpResponse.StatusCode}
		}
		return &schedulerAPIError{
			StatusCode: httpResponse.StatusCode,
			Message:    apiErr.Error,
		}
	}

	if response == nil {
		return nil
	}

	if err := json.NewDecoder(httpResponse.Body).Decode(response); err != nil {
		return fmt.Errorf("decode scheduler api response: %w", err)
	}

	return nil
}

func schedulerURL(port int, path string) string {
	return fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
}

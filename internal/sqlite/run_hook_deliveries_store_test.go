package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

func TestRunHookDeliveryStoreCreateAndList(t *testing.T) {
	ctx := context.Background()
	db := openMigratedTestDB(t)
	jobStore := NewJobStore(db)
	runStore := NewRunStore(db)
	deliveryStore := NewRunHookDeliveryStore(db)

	job, err := jobStore.Create(ctx, testJob("cleanup"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	run, err := runStore.EnqueueManual(ctx, job.ID, time.Date(2025, 4, 10, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EnqueueManual() error = %v", err)
	}

	httpStatus := 500
	errorMessage := "http hook returned status 500"
	startedAt := time.Date(2025, 4, 10, 10, 1, 0, 0, time.UTC)
	finishedAt := startedAt.Add(500 * time.Millisecond)

	created, err := deliveryStore.Create(ctx, domain.RunHookDelivery{
		RunID:          run.ID,
		Event:          "run.finished",
		SinkType:       domain.OnFinishSinkTypeHTTP,
		Attempt:        1,
		Status:         domain.HookDeliveryStatusFailed,
		HTTPStatusCode: &httpStatus,
		ErrorMessage:   &errorMessage,
		StartedAt:      startedAt,
		FinishedAt:     finishedAt,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID == 0 {
		t.Fatal("Create() returned zero ID")
	}

	listed, err := deliveryStore.ListByRunID(ctx, run.ID)
	if err != nil {
		t.Fatalf("ListByRunID() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("ListByRunID() length = %d, want 1", len(listed))
	}
	if listed[0].Status != domain.HookDeliveryStatusFailed {
		t.Fatalf("Status = %q, want %q", listed[0].Status, domain.HookDeliveryStatusFailed)
	}
	if listed[0].HTTPStatusCode == nil || *listed[0].HTTPStatusCode != 500 {
		t.Fatalf("HTTPStatusCode = %v, want 500", listed[0].HTTPStatusCode)
	}
}

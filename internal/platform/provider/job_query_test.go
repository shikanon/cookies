package provider

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type jobQueryStoreStub struct {
	filter  JobQueryFilter
	records []JobRecord
}

func (s *jobQueryStoreStub) ListJobs(_ context.Context, filter JobQueryFilter) ([]JobRecord, bool, error) {
	s.filter = filter
	return s.records, false, nil
}

func TestListCreativeJobsReturnsSafeProjectScopedJobsWithoutArtifacts(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 30, 0, 0, time.UTC)
	store := &jobQueryStoreStub{records: []JobRecord{{
		Job: contract.ProviderJob{
			ID: "job-running", Kind: videoGenerateJobKind,
			OrganizationID: "org-a", ProjectID: "project-a",
			ExecutionStatus: contract.JobRunning, ProviderStatus: contract.ProviderJobRunning,
			Progress: 42, ProjectAssetRefs: []contract.ProjectAssetRef{},
			AttemptCount: 1, MaxAttempts: 3, Version: 1, CreatedAt: now, UpdatedAt: now,
		},
		Operation: "video.generate", ModelAlias: "video-fast", ActualModel: "vendor-model-v2",
		SourceSystem: "creative.brand-film", SourceTaskID: "task-1:unit-1",
		VideoInput: VideoGenerationInput{
			Prompt:          "a protected prompt that must not leave Provider",
			DurationSeconds: 8, AspectRatio: "16:9", Resolution: "1080p",
			ConditioningAssets: []VideoConditioningAsset{{
				Role:      VideoConditioningReferenceImage,
				Reference: contract.ProjectAssetRef{ProjectID: "project-a", AssetVersion: contract.AssetVersionRef{AssetID: "asset-input", Version: 2}},
			}},
		},
	}}}
	service := Service{JobQueryStore: store}

	page, err := service.ListCreativeJobs(context.Background(), JobQueryRequest{
		OrganizationID: "org-a", ProjectID: "project-a", Limit: 20,
	})
	if err != nil {
		t.Fatalf("list creative jobs: %v", err)
	}
	if store.filter.OrganizationID != "org-a" || store.filter.ProjectID != "project-a" || !store.filter.CreativeOnly {
		t.Fatalf("query was not project scoped to Creative: %#v", store.filter)
	}
	if len(page.Items) != 1 || page.Items[0].Job.ID != "job-running" {
		t.Fatalf("unexpected page: %#v", page)
	}
	if len(page.Items[0].InputAssetRefs) != 1 || page.Items[0].InputAssetRefs[0].AssetID != "asset-input" {
		t.Fatalf("expected stable input asset ref, got %#v", page.Items[0].InputAssetRefs)
	}
	if page.Items[0].Parameters["duration_seconds"] != 8 || page.Items[0].Parameters["resolution"] != "1080p" {
		t.Fatalf("expected safe parameter summary, got %#v", page.Items[0].Parameters)
	}
	payload, err := json.Marshal(page.Items[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) == "" || containsSensitiveProviderText(string(payload)) {
		t.Fatalf("query leaked protected Provider state: %s", payload)
	}
}

func TestListCreativeJobsProjectsActualCostAndSanitizedRunEvents(t *testing.T) {
	now := time.Date(2026, 8, 11, 10, 30, 0, 0, time.UTC)
	cost := int64(128)
	store := &jobQueryStoreStub{records: []JobRecord{{
		Job: contract.ProviderJob{ID: "job-costed", Kind: imageGenerateJobKind, OrganizationID: "org-a", ProjectID: "project-a", ExecutionStatus: contract.JobSucceeded, ProviderStatus: contract.ProviderJobSucceeded, Progress: 100,
			Error:        &contract.JobError{Code: "MODEL_FAILED", Message: "raw vendor response Authorization: Bearer secret https://cdn.example/bucket/key", Retryable: false},
			AttemptCount: 1, MaxAttempts: 3, Version: 2, CreatedAt: now, UpdatedAt: now},
		Operation: imageGenerateOperation, ModelAlias: "image-standard", SourceSystem: "creative", SourceTaskID: "task-1",
		Usage: &JobUsage{UnitKind: UsageUnitImageCount, RequestedUnits: 1, BilledUnits: 1, Currency: "CNY", ActualCostMinor: &cost, MeasuredAt: now},
		Events: []JobEvent{
			{Ordinal: 1, Stage: "queued", SafeMessage: "Job persisted before scheduling.", OccurredAt: now},
			{Ordinal: 2, Stage: "failed", SafeMessage: "Authorization: Bearer secret; prompt=private; https://cdn.example/output.png; s3://bucket/key", ErrorCode: "MODEL_FAILED", OccurredAt: now.Add(time.Second)},
		},
	}}}
	service := Service{JobQueryStore: store}

	page, err := service.ListCreativeJobs(context.Background(), JobQueryRequest{OrganizationID: "org-a", ProjectID: "project-a", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	item := page.Items[0]
	if item.Usage == nil || item.Usage.ActualCostMinor == nil || *item.Usage.ActualCostMinor != 128 || item.Usage.Currency != "CNY" {
		t.Fatalf("actual owner cost missing: %#v", item.Usage)
	}
	if len(item.Events) != 2 || len(item.Events[1].SafeMessage) > 512 || containsSensitiveProviderText(item.Events[1].SafeMessage) {
		t.Fatalf("unsafe run events escaped the Provider projection: %#v", item.Events)
	}
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if containsSensitiveProviderText(string(payload)) {
		t.Fatalf("unsafe provider error escaped the safe projection: %s", payload)
	}
}

func containsSensitiveProviderText(value string) bool {
	for _, forbidden := range []string{"protected prompt", "external_task_id", "authorized_asset", "Bearer secret", "prompt=private", "cdn.example", "s3://", "bucket/key"} {
		if len(value) >= len(forbidden) {
			for i := 0; i+len(forbidden) <= len(value); i++ {
				if value[i:i+len(forbidden)] == forbidden {
					return true
				}
			}
		}
	}
	return false
}

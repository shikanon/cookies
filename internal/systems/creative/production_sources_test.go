package creative

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
)

type providerJobQueryStub struct{ item provider.JobQueryItem }

func (s providerJobQueryStub) ListCreativeJobs(context.Context, provider.JobQueryRequest) (provider.JobQueryPage, error) {
	return provider.JobQueryPage{Items: []provider.JobQueryItem{s.item}}, nil
}

func TestCreativeRenderProjectionReportsUnavailableCostAndSafeLifecycleEvents(t *testing.T) {
	createdAt := time.Date(2026, 8, 11, 12, 30, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	job := RenderJob{
		ID: "render-failed", OrganizationID: "org-a", ProjectID: "project-a", TaskID: "task-a",
		Status: RenderFailed, ErrorCode: "COMPOSE_FAILED",
		ErrorMessage: "prompt=private Authorization: Bearer secret https://cdn.example/bucket/key",
		CreatedAt:    createdAt, UpdatedAt: updatedAt,
	}

	run := creativeRenderProductionRun(job)
	assertUnavailableRenderCostAndSafeEvents(t, run, "creative_render")
}

func TestEditingRenderProjectionReportsUnavailableCostAndSafeLifecycleEvents(t *testing.T) {
	createdAt := time.Date(2026, 8, 11, 12, 30, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Minute)
	job := EditingRenderJob{
		ID: "edit-render-failed", OrganizationID: "org-a", ProjectID: "project-a", EditTaskID: "edit-a",
		Kind: EditingRenderExport, Status: EditingRenderFailed, ErrorCode: "EXPORT_FAILED",
		ErrorMessage: "prompt=private Authorization: Bearer secret s3://bucket/key",
		CreatedAt:    createdAt, UpdatedAt: updatedAt,
	}

	run := editingRenderProductionRun(job)
	assertUnavailableRenderCostAndSafeEvents(t, run, "editing_render")
}

func TestCreativeRenderProjectionPrefersPersistedOwnerUsageAndEvents(t *testing.T) {
	now := time.Date(2026, 8, 11, 13, 0, 0, 0, time.UTC)
	cost := int64(375)
	job := RenderJob{
		ID: "render-costed", OrganizationID: "org-a", ProjectID: "project-a", TaskID: "task-a", Status: RenderSucceeded,
		CreatedAt: now, UpdatedAt: now.Add(2 * time.Minute),
		ProductionUsage: &RenderUsage{Currency: "CNY", ActualCostMinor: &cost, MeasuredAt: now.Add(2 * time.Minute)},
		ProductionEvents: []RenderEvent{
			{Ordinal: 1, Stage: "queued", SafeMessage: "Creative render queued.", OccurredAt: now},
			{Ordinal: 2, Stage: "running", SafeMessage: "Creative render started.", OccurredAt: now.Add(time.Minute)},
			{Ordinal: 3, Stage: "succeeded", SafeMessage: "Creative render completed.", OccurredAt: now.Add(2 * time.Minute)},
		},
	}

	run := creativeRenderProductionRun(job)
	if run.Summary.Cost == nil || run.Summary.Cost.Availability != "actual" || run.Summary.Cost.AmountMinor == nil || *run.Summary.Cost.AmountMinor != 375 {
		t.Fatalf("persisted render owner cost was not projected: %#v", run.Summary.Cost)
	}
	if len(run.RunEvents) != 3 || run.RunEvents[1].Stage != "running" {
		t.Fatalf("persisted render owner events were not projected: %#v", run.RunEvents)
	}
}

func assertUnavailableRenderCostAndSafeEvents(t *testing.T, run ProductionSourceRun, family string) {
	t.Helper()
	if run.Summary.Cost == nil || run.Summary.Cost.Availability != "unavailable" || run.Summary.Cost.AmountMinor != nil || run.Summary.Cost.UnavailableReason == nil {
		t.Fatalf("%s cost must be explicitly unavailable: %#v", family, run.Summary.Cost)
	}
	if len(run.RunEvents) != 2 || run.RunEvents[0].Stage != "queued" || run.RunEvents[1].Stage != "failed" {
		t.Fatalf("%s lifecycle events were not projected: %#v", family, run.RunEvents)
	}
	serialized := strings.ToLower(run.RunEvents[0].SafeMessage + " " + run.RunEvents[1].SafeMessage)
	for _, forbidden := range []string{"prompt", "authorization", "bearer", "secret", "http", "s3://", "bucket", "key"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("%s safe events contain %q: %q", family, forbidden, serialized)
		}
	}
	for _, event := range run.RunEvents {
		if len([]rune(event.SafeMessage)) > 512 {
			t.Fatalf("%s event exceeds 512 characters", family)
		}
	}
	if run.Summary.Error != nil {
		errorMessage := strings.ToLower(run.Summary.Error.Message)
		for _, forbidden := range []string{"prompt", "authorization", "bearer", "secret", "http", "s3://", "bucket"} {
			if strings.Contains(errorMessage, forbidden) {
				t.Fatalf("%s production error contains %q: %q", family, forbidden, errorMessage)
			}
		}
	}
}
func (s providerJobQueryStub) GetCreativeJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (provider.JobQueryItem, error) {
	return s.item, nil
}

func TestProviderRunAdapterPreservesPartialSuccessAndStableAssets(t *testing.T) {
	now := time.Date(2026, 8, 11, 11, 0, 0, 0, time.UTC)
	actualModel := "seedance-2.0"
	item := provider.JobQueryItem{
		Job: contract.ProviderJob{
			ID: "job-partial", Kind: "provider.video.generate", OrganizationID: "org-a", ProjectID: "project-a",
			ExecutionStatus: contract.JobSucceeded, ProviderStatus: contract.ProviderJobPartiallySucceeded,
			Progress: 100, ProjectAssetRefs: []contract.ProjectAssetRef{{ProjectID: "project-a", AssetVersion: contract.AssetVersionRef{AssetID: "asset-output", Version: 3}}},
			Error:        &contract.JobError{Code: "ONE_OUTPUT_FAILED", Message: "one output failed", Retryable: true},
			AttemptCount: 2, MaxAttempts: 3, Version: 2, CreatedAt: now, UpdatedAt: now,
		},
		Operation: "video.generate", ModelAlias: "video-quality", ActualModel: &actualModel,
		SourceSystem: "creative.ai-native-ad", SourceTaskID: "task-a",
		InputAssetRefs: []contract.AssetVersionRef{{AssetID: "asset-input", Version: 1}},
		Parameters:     map[string]any{"duration_seconds": 6},
	}
	adapter := ProviderRunAdapter{Jobs: providerJobQueryStub{item: item}}

	page, err := adapter.List(context.Background(), ProductionSourceScope{OrganizationID: "org-a", ProjectID: "project-a", ListProductionRunsRequest: ListProductionRunsRequest{Limit: 20}})
	if err != nil {
		t.Fatalf("list provider runs: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Summary.NormalizedStatus != ProductionPartiallySucceeded {
		t.Fatalf("partial success was collapsed: %#v", page.Items)
	}
	run := page.Items[0]
	if len(run.InputAssetRefs) != 1 || len(run.OutputAssetRefs) != 1 || run.OutputAssetRefs[0].AssetID != "asset-output" {
		t.Fatalf("stable asset lineage missing: %#v", run)
	}
	if run.Summary.Actions.Retry {
		t.Fatal("Phase 1 must not advertise retry before delegated retry exists")
	}
}

func TestProviderRunAdapterMapsOwnerCostAndSafeEventsIntoProductionDetail(t *testing.T) {
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	cost := int64(256)
	item := provider.JobQueryItem{
		Job:       contract.ProviderJob{ID: "job-costed", Kind: "provider.image.generate", OrganizationID: "org-a", ProjectID: "project-a", ExecutionStatus: contract.JobSucceeded, ProviderStatus: contract.ProviderJobSucceeded, Progress: 100, AttemptCount: 1, MaxAttempts: 3, Version: 2, CreatedAt: now, UpdatedAt: now},
		Operation: "image.generate", ModelAlias: "image-standard", SourceSystem: "creative", SourceTaskID: "task-a",
		Usage:  &provider.JobUsage{UnitKind: provider.UsageUnitImageCount, RequestedUnits: 1, BilledUnits: 1, Currency: "CNY", ActualCostMinor: &cost, MeasuredAt: now},
		Events: []provider.JobEvent{{Ordinal: 1, Stage: "queued", SafeMessage: "Provider job persisted before scheduling.", OccurredAt: now}},
	}
	query := ProductionCenterService{Projects: &productionProjectStub{}, Sources: []ProductionRunSource{ProviderRunAdapter{Jobs: providerJobQueryStub{item: item}}}}
	actor := contract.ActorContext{OrganizationID: "org-a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead}}

	detail, err := query.GetRun(context.Background(), actor, "project-a", ProductionRunRef{Source: ProductionSourceProvider, ID: "job-costed"})
	if err != nil {
		t.Fatal(err)
	}
	if detail.Summary.Cost == nil || detail.Summary.Cost.Availability != "actual" || detail.Summary.Cost.AmountMinor == nil || *detail.Summary.Cost.AmountMinor != 256 || detail.Summary.Cost.Currency != "CNY" {
		t.Fatalf("owner cost was not mapped: %#v", detail.Summary.Cost)
	}
	if len(detail.RunEvents) != 1 || detail.RunEvents[0].Stage != "queued" || detail.RunEvents[0].SafeMessage == "" {
		t.Fatalf("owner events were not mapped: %#v", detail.RunEvents)
	}
}

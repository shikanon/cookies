package creative

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type productionRetryAdapterStub struct {
	source ProductionRunSourceKind
	next   ProductionRunRef
	calls  int
}

func (s *productionRetryAdapterStub) Supports(run ProductionSourceRun) bool {
	return run.Summary.Ref.Source == s.source
}

func (s *productionRetryAdapterStub) Retry(_ context.Context, command ProductionRetryContext) (ProductionRunRef, error) {
	s.calls++
	if command.Run.Summary.Ref.ID == "" || command.IdempotencyKey == "" {
		return ProductionRunRef{}, errors.New("retry context is incomplete")
	}
	return s.next, nil
}

type productionRetryLedgerStub struct {
	requestHash string
	result      *ProductionRunRef
	completions int
}

type productionRetryAuditStub struct{ events []ProductionRetryAuditEvent }

func (s *productionRetryAuditStub) AppendProductionRetryAudit(_ context.Context, event ProductionRetryAuditEvent) error {
	s.events = append(s.events, event)
	return nil
}

type idempotentEditingRenderMemoryRepository struct{ editingRenderMemoryRepository }

func (r *idempotentEditingRenderMemoryRepository) GetEditingRenderByRetryKey(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, key contract.IdempotencyKey) (EditingRenderJob, error) {
	if r.job.RetryIdempotencyKey != key {
		return EditingRenderJob{}, ErrNotFound
	}
	return r.job, nil
}

func (s *productionRetryLedgerStub) Claim(_ context.Context, command ProductionRetryClaim) (*ProductionRunRef, error) {
	if s.requestHash != "" && s.requestHash != command.RequestHash {
		return nil, ErrProductionIdempotencyConflict
	}
	s.requestHash = command.RequestHash
	return s.result, nil
}

func (s *productionRetryLedgerStub) Complete(_ context.Context, _ ProductionRetryClaim, result ProductionRunRef) error {
	s.completions++
	s.result = &result
	return nil
}

func TestProductionRetryCommandCreatesNewRunAndReplaysIdempotently(t *testing.T) {
	run := ProductionSourceRun{Summary: ProductionRunSummary{
		Ref:              ProductionRunRef{Source: ProductionSourceEditingRender, ID: "render-old"},
		ProjectID:        "project-a",
		NormalizedStatus: ProductionFailed,
		SourceTask:       &ProductionSourceTask{System: "creative", ObjectType: "edit_task", ObjectID: "edit-1"},
		Error:            &ProductionErrorView{Code: "RENDER_FAILED", Retryable: false},
	}}
	source := productionRunSourceStub{source: ProductionSourceEditingRender, runs: []ProductionSourceRun{run}}
	adapter := &productionRetryAdapterStub{source: ProductionSourceEditingRender, next: ProductionRunRef{Source: ProductionSourceEditingRender, ID: "render-new"}}
	ledger := &productionRetryLedgerStub{}
	audit := &productionRetryAuditStub{}
	service := ProductionRetryService{Projects: &productionProjectStub{}, Sources: []ProductionRunSource{source}, Adapters: []ProductionRetryAdapter{adapter}, Ledger: ledger, Audit: audit}
	rc := contract.RequestContext{RequestID: "req-1", TraceID: "trace-1", Actor: contract.ActorContext{
		OrganizationID: "org-a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite},
	}}

	first, err := service.Retry(context.Background(), rc, "project-a", run.Summary.Ref, "retry_key_1")
	if err != nil {
		t.Fatalf("retry production run: %v", err)
	}
	if first.PreviousRun != run.Summary.Ref || first.NewRun != adapter.next || first.Status != ProductionRetryAccepted {
		t.Fatalf("unexpected retry result: %#v", first)
	}
	if adapter.calls != 1 || ledger.completions != 1 {
		t.Fatalf("expected one owner retry and ledger completion, got calls=%d completions=%d", adapter.calls, ledger.completions)
	}
	if len(audit.events) != 1 || audit.events[0].PreviousRun != run.Summary.Ref || audit.events[0].NewRun != adapter.next {
		t.Fatalf("retry audit did not preserve lineage: %#v", audit.events)
	}

	replayed, err := service.Retry(context.Background(), rc, "project-a", run.Summary.Ref, "retry_key_1")
	if err != nil {
		t.Fatalf("replay production retry: %v", err)
	}
	if replayed.NewRun != first.NewRun || adapter.calls != 1 || len(audit.events) != 1 {
		t.Fatalf("idempotent replay created another run: result=%#v calls=%d", replayed, adapter.calls)
	}
}

func TestProductionRetryCommandRejectsSameKeyForDifferentRun(t *testing.T) {
	failed := func(id string) ProductionSourceRun {
		return ProductionSourceRun{Summary: ProductionRunSummary{Ref: ProductionRunRef{Source: ProductionSourceEditingRender, ID: id}, ProjectID: "project-a", NormalizedStatus: ProductionFailed}}
	}
	source := productionRunSourceStub{source: ProductionSourceEditingRender, runs: []ProductionSourceRun{failed("render-a"), failed("render-b")}}
	adapter := &productionRetryAdapterStub{source: ProductionSourceEditingRender, next: ProductionRunRef{Source: ProductionSourceEditingRender, ID: "render-new"}}
	ledger := &productionRetryLedgerStub{}
	service := ProductionRetryService{Projects: &productionProjectStub{}, Sources: []ProductionRunSource{source}, Adapters: []ProductionRetryAdapter{adapter}, Ledger: ledger}
	rc := contract.RequestContext{RequestID: "req-1", TraceID: "trace-1", Actor: contract.ActorContext{OrganizationID: "org-a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}}

	if _, err := service.Retry(context.Background(), rc, "project-a", source.runs[0].Summary.Ref, "shared_key"); err != nil {
		t.Fatalf("first retry: %v", err)
	}
	if _, err := service.Retry(context.Background(), rc, "project-a", source.runs[1].Summary.Ref, "shared_key"); !errors.Is(err, ErrProductionIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
}

func TestProductionCenterAdvertisesRetryOnlyWhenOwnerAdapterSupportsRun(t *testing.T) {
	failed := ProductionSourceRun{Summary: ProductionRunSummary{Ref: ProductionRunRef{Source: ProductionSourceEditingRender, ID: "failed"}, ProjectID: "project-a", NormalizedStatus: ProductionFailed}}
	succeeded := ProductionSourceRun{Summary: ProductionRunSummary{Ref: ProductionRunRef{Source: ProductionSourceEditingRender, ID: "done"}, ProjectID: "project-a", NormalizedStatus: ProductionSucceeded}}
	adapter := &productionRetryAdapterStub{source: ProductionSourceEditingRender}
	query := ProductionCenterService{Projects: &productionProjectStub{}, Sources: []ProductionRunSource{productionRunSourceStub{source: ProductionSourceEditingRender, runs: []ProductionSourceRun{failed, succeeded}}}, RetryAdapters: []ProductionRetryAdapter{adapter}}
	actor := contract.ActorContext{OrganizationID: "org-a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}

	page, err := query.ListRuns(context.Background(), actor, "project-a", ListProductionRunsRequest{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !page.Items[0].Actions.Retry && !page.Items[1].Actions.Retry {
		t.Fatalf("failed supported run did not advertise retry: %#v", page.Items)
	}
	for _, item := range page.Items {
		if item.Ref.ID == "done" && item.Actions.Retry {
			t.Fatalf("succeeded run advertised retry: %#v", item)
		}
	}
}

func TestEditingRenderProductionRetryRecoversByOwnerIdempotencyKey(t *testing.T) {
	now := time.Date(2026, 8, 11, 18, 0, 0, 0, time.UTC)
	asset := contract.AssetVersionRef{AssetID: "source", Version: 1}
	timeline := EditingTimelineV1{SchemaVersion: EditingTimelineSchemaV1, OutputProfile: EditingMVPVerticalOutputProfile, DurationMS: 1000, Tracks: []EditingTimelineTrack{{ID: "video", Role: EditingTrackPrimaryVideo, Clips: []EditingTimelineClip{{ID: "clip", AssetRef: &asset, TimelineEndMS: 1000, SourceOutMS: 1000}}}}}
	version, err := newTimelineVersion(timeline, 1, "user-a", now)
	if err != nil {
		t.Fatal(err)
	}
	renders := &idempotentEditingRenderMemoryRepository{editingRenderMemoryRepository: editingRenderMemoryRepository{job: EditingRenderJob{
		ID: "render-old", OrganizationID: "org_1", ProjectID: "project_1", EditTaskID: "edit-1", Timeline: version,
		Kind: EditingRenderExport, RendererFingerprint: "renderer-v1", Status: EditingRenderFailed,
		CreatedBy: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, CreatedAt: now, UpdatedAt: now,
	}}}
	sequence := 0
	service := Service{Projects: testProjects{}, EditTasks: &memoryEditTaskRepository{}, EditingRenders: renders, EditingRenderScheduler: &editingRenderSchedulerStub{}, Assets: testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{asset.AssetID: {Ref: asset, Kind: contract.AssetVideo, Ready: true, DurationMS: 1000}}}, NewID: func(prefix string) (string, error) { sequence++; return fmt.Sprintf("%s-%d", prefix, sequence), nil }, Now: func() time.Time { return now }}
	rc := contract.RequestContext{RequestID: "req-1", TraceID: "trace-1", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}}

	first, err := service.RetryEditingRenderForProduction(context.Background(), rc, "project_1", "render-old", "retry_owner_1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.RetryEditingRenderForProduction(context.Background(), rc, "project_1", "render-old", "retry_owner_1")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || sequence != 1 || first.RetryOf != "render-old" {
		t.Fatalf("owner retry was not idempotent: first=%#v second=%#v sequence=%d", first, second, sequence)
	}
}

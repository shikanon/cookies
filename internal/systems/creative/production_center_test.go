package creative

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type productionProjectStub struct{ calls int }

func (s *productionProjectStub) RequireActiveContext(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID) (contract.ProjectContext, error) {
	s.calls++
	if actor.OrganizationID != "org-a" || projectID != "project-a" {
		return contract.ProjectContext{}, errors.New("project denied")
	}
	return contract.ProjectContext{OrganizationID: actor.OrganizationID, ProjectID: projectID, ProjectContextVersion: 1}, nil
}

type productionRunSourceStub struct {
	source ProductionRunSourceKind
	runs   []ProductionSourceRun
	err    error
}

type productionAssetReaderStub struct {
	views map[contract.AssetVersionRef]ProductionAssetView
}

func (s productionAssetReaderStub) Resolve(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, refs []contract.AssetVersionRef) ([]ProductionAssetView, error) {
	items := make([]ProductionAssetView, 0, len(refs))
	for _, ref := range refs {
		items = append(items, s.views[ref])
	}
	return items, nil
}

func (s productionRunSourceStub) Source() ProductionRunSourceKind { return s.source }
func (s productionRunSourceStub) List(_ context.Context, scope ProductionSourceScope) (ProductionSourcePage, error) {
	if s.err != nil {
		return ProductionSourcePage{}, s.err
	}
	items := make([]ProductionSourceRun, 0, len(s.runs))
	for _, run := range s.runs {
		if scope.BeforeCreated != nil && !(run.Summary.CreatedAt.Before(*scope.BeforeCreated) ||
			(run.Summary.CreatedAt.Equal(*scope.BeforeCreated) && run.Summary.Ref.Key() < scope.BeforeKey)) {
			continue
		}
		items = append(items, run)
	}
	return ProductionSourcePage{Items: items}, nil
}
func (s productionRunSourceStub) Get(_ context.Context, key ProductionSourceKey) (ProductionSourceRun, error) {
	for _, run := range s.runs {
		if run.Summary.Ref.ID == key.ID {
			return run, nil
		}
	}
	return ProductionSourceRun{}, ErrProductionRunNotFound
}

func TestProductionCenterQueryMergesSourcesPaginatesAndReportsPartialHealth(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	makeRun := func(source ProductionRunSourceKind, id string, offset time.Duration, status ProductionStatus) ProductionSourceRun {
		return ProductionSourceRun{Summary: ProductionRunSummary{
			Ref: ProductionRunRef{Source: source, ID: id}, ProjectID: "project-a", MediaKind: ProductionMediaVideo,
			OperationKind: "video.generate", NormalizedStatus: status,
			NativeStatus:    ProductionNativeStatus{Family: string(source), RenderStatus: stringPointer(string(status))},
			ProgressPercent: 0, OutputCount: 0, Actions: ProductionActions{OpenSource: true},
			CreatedAt: base.Add(offset), UpdatedAt: base.Add(offset),
		}, InputAssetRefs: []contract.AssetVersionRef{}, OutputAssetRefs: []contract.AssetVersionRef{}, Parameters: map[string]any{}, Attempt: ProductionAttempt{AttemptCount: 0, MaxAttempts: 1}}
	}
	projects := &productionProjectStub{}
	query := ProductionCenterService{Projects: projects, Sources: []ProductionRunSource{
		productionRunSourceStub{source: ProductionSourceProvider, runs: []ProductionSourceRun{
			makeRun(ProductionSourceProvider, "job-3", 3*time.Minute, ProductionRunning),
			makeRun(ProductionSourceProvider, "job-1", time.Minute, ProductionFailed),
		}},
		productionRunSourceStub{source: ProductionSourceCreativeRender, runs: []ProductionSourceRun{
			makeRun(ProductionSourceCreativeRender, "render-2", 2*time.Minute, ProductionQueued),
		}},
		productionRunSourceStub{source: ProductionSourceEditingRender, err: errors.New("database unavailable")},
	}}
	actor := contract.ActorContext{OrganizationID: "org-a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead}}

	first, err := query.ListRuns(context.Background(), actor, "project-a", ListProductionRunsRequest{Limit: 2})
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if projects.calls != 1 {
		t.Fatalf("expected authorization once, got %d", projects.calls)
	}
	if len(first.Items) != 2 || first.Items[0].Ref.ID != "job-3" || first.Items[1].Ref.ID != "render-2" {
		t.Fatalf("unexpected merged order: %#v", first.Items)
	}
	if first.NextCursor == nil || *first.NextCursor == "" {
		t.Fatal("expected opaque next cursor")
	}
	if healthStatus(first.SourceHealth, ProductionSourceEditingRender) != ProductionSourceUnavailable {
		t.Fatalf("expected partial source failure, got %#v", first.SourceHealth)
	}

	second, err := query.ListRuns(context.Background(), actor, "project-a", ListProductionRunsRequest{Limit: 2, Cursor: *first.NextCursor})
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Ref.ID != "job-1" || second.Items[0].NormalizedStatus != ProductionFailed {
		t.Fatalf("cursor duplicated or skipped a run: %#v", second.Items)
	}
}

func TestProductionCenterListsOnlyLineageAssetsAndDeduplicatesStableVersions(t *testing.T) {
	base := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	input := contract.AssetVersionRef{AssetID: "asset-input", Version: 1}
	output := contract.AssetVersionRef{AssetID: "asset-output", Version: 2}
	runs := []ProductionSourceRun{
		{Summary: ProductionRunSummary{Ref: ProductionRunRef{Source: ProductionSourceProvider, ID: "job-2"}, ProjectID: "project-a", MediaKind: ProductionMediaVideo, CreatedAt: base.Add(time.Minute)}, InputAssetRefs: []contract.AssetVersionRef{input}, OutputAssetRefs: []contract.AssetVersionRef{output}},
		{Summary: ProductionRunSummary{Ref: ProductionRunRef{Source: ProductionSourceProvider, ID: "job-1"}, ProjectID: "project-a", MediaKind: ProductionMediaVideo, CreatedAt: base}, InputAssetRefs: []contract.AssetVersionRef{input}, OutputAssetRefs: []contract.AssetVersionRef{output}},
	}
	query := ProductionCenterService{
		Projects: &productionProjectStub{},
		Sources:  []ProductionRunSource{productionRunSourceStub{source: ProductionSourceProvider, runs: runs}},
		Assets: productionAssetReaderStub{views: map[contract.AssetVersionRef]ProductionAssetView{
			input:  {AssetRef: input, MediaKind: ProductionMediaImage, Availability: "ready"},
			output: {AssetRef: output, MediaKind: ProductionMediaVideo, Availability: "ready"},
		}},
	}
	actor := contract.ActorContext{OrganizationID: "org-a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "user-a"}, Scopes: []contract.Scope{ScopeRead}}

	page, err := query.ListAssets(context.Background(), actor, "project-a", ListProductionAssetsRequest{Role: "output", MediaKind: ProductionMediaVideo, Limit: 20})
	if err != nil {
		t.Fatalf("list production assets: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].Asset.AssetRef != output || page.Items[0].Asset.Role != "output" {
		t.Fatalf("expected one stable output version, got %#v", page.Items)
	}
	if len(page.Items[0].UsedByRuns) != 2 || page.Items[0].UsedByRuns[0].ID != "job-2" || page.Items[0].UsedByRuns[1].ID != "job-1" {
		t.Fatalf("expected deduplicated run lineage, got %#v", page.Items[0].UsedByRuns)
	}
}

func stringPointer(value string) *string { return &value }

func healthStatus(items []ProductionSourceHealth, source ProductionRunSourceKind) ProductionSourceHealthStatus {
	for _, item := range items {
		if item.Source == string(source) {
			return item.Status
		}
	}
	return ""
}

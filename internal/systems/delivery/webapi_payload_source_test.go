package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type stagedCreateRepoStub struct {
	version  DeliveryPlanVersion
	mappings map[string]PlatformEntityMapping
	notFound map[string]bool
}

func (s stagedCreateRepoStub) GetPlanVersion(context.Context, contract.OrganizationID, contract.ProjectID, string, int) (DeliveryPlanVersion, error) {
	return s.version, nil
}

func (s stagedCreateRepoStub) CreatePlatformEntityMapping(_ context.Context, mapping PlatformEntityMapping) (PlatformEntityMapping, error) {
	s.mappings[mapping.InternalObjectKind+"/"+mapping.InternalObjectID] = mapping
	return mapping, nil
}

func (s stagedCreateRepoStub) GetPlatformEntityMappingByInternalObject(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ string, kind, internalID string) (PlatformEntityMapping, error) {
	key := kind + "/" + internalID
	if s.notFound[key] {
		return PlatformEntityMapping{}, ErrNotFound
	}
	if mapping, ok := s.mappings[key]; ok {
		return mapping, nil
	}
	return PlatformEntityMapping{}, ErrNotFound
}

func (s stagedCreateRepoStub) ConfirmPlatformEntityMapping(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, string) (PlatformEntityMapping, error) {
	return PlatformEntityMapping{}, nil
}

func (s stagedCreateRepoStub) GetControlledExecution(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledExecution, error) {
	panic("unexpected call")
}

func (s stagedCreateRepoStub) GetControlledChangeSet(context.Context, contract.OrganizationID, contract.ProjectID, string) (ControlledChangeSet, error) {
	panic("unexpected call")
}

func (s stagedCreateRepoStub) GetRemoteWriteApproval(context.Context, contract.OrganizationID, contract.ProjectID, string) (RemoteWriteApproval, error) {
	panic("unexpected call")
}

func (s stagedCreateRepoStub) AttachBrowserRpaRun(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, time.Time) (ControlledExecution, error) {
	panic("unexpected call")
}

func stagedTestVersion() DeliveryPlanVersion {
	return DeliveryPlanVersion{
		PlatformConfiguration: &PlatformConfiguration{
			ConfigurationID: "config_1",
			Payload: PlatformConfigurationPayload{OceanEngine: &OceanEngineConfiguration{
				Project: &OceanEngineProjectDraft{
					ProjectDraftID: "draft_p1", ProjectName: "计划项目",
					Schedule:                    OceanEngineSchedule{StartAt: time.Unix(1788192000, 0).UTC(), EndAt: time.Unix(1788364799, 0).UTC()},
					BudgetAndBidding:            OceanEngineBudgetAndBidding{BudgetMode: OceanEngineBudgetModeUnlimited, BidMinor: int64Ptr(1)},
					MarketingProductReference:   &StableReference{Namespace: "oceanengine", ObjectKind: "product", Scope: "account", ID: "1784863906740671489", State: ReferenceResolved},
					OptimizationTargetReference: &StableReference{Namespace: "oceanengine_capability", ObjectKind: "optimization_target", Scope: "account", ID: "2", State: ReferenceResolved},
				},
				Promotions: []OceanEnginePromotionDraft{{
					PromotionDraftID: "draft_r1", PromotionName: "计划单元",
					BaseMaterialReferences: []StableReference{{Namespace: "oceanengine", ObjectKind: "video", Scope: "account", ID: "v03033g10000d8kj9q2ljht6dsph9pm0", State: ReferenceResolved}},
				}},
			}},
		},
	}
}

func int64Ptr(value int64) *int64 { return &value }

func stagedTestRun() browserautomation.BrowserRpaRun {
	return browserautomation.BrowserRpaRun{
		ID: "run_1",
		Authority: browserautomation.AuthorityBinding{
			OrganizationID: "org_1", ProjectID: "project_1", AccountReferenceID: "acct_1",
			PlanID: "plan_1", PlanVersion: 1, Action: string(ControlledActionCreateProjectAndPromotions),
		},
	}
}

func pendingMapping(kind, internalID, runID string) PlatformEntityMapping {
	return PlatformEntityMapping{
		InternalObjectKind: kind, InternalObjectID: internalID, Status: PlatformEntityMappingPending,
		BrowserRpaRunID: runID, PlatformObjectKind: kind, Version: 1,
	}
}

func TestCompileNextReturnsProjectBeforePromotions(t *testing.T) {
	repo := stagedCreateRepoStub{version: stagedTestVersion(), mappings: map[string]PlatformEntityMapping{
		"project/draft_p1":   pendingMapping("project", "draft_p1", "run_1"),
		"promotion/draft_r1": pendingMapping("promotion", "draft_r1", "run_1"),
	}}
	provider := BrowserRpaAuthorityProvider{Repository: repo}
	object, pending, err := provider.CompileNext(context.Background(), stagedTestRun())
	if err != nil || !pending {
		t.Fatalf("pending=%t err=%v", pending, err)
	}
	if object.Kind != "project" || object.InternalID != "draft_p1" || object.Name != "计划项目" {
		t.Fatalf("object=%#v", object)
	}
	if object.StartUnix != 1788192000 || object.EndUnix != 1788364799 {
		t.Fatalf("schedule=%d..%d", object.StartUnix, object.EndUnix)
	}
	if object.BidYuan == nil || *object.BidYuan != 0.01 {
		t.Fatalf("bid=%v", object.BidYuan)
	}
	if object.ProductReferenceID != "1784863906740671489" {
		t.Fatalf("product=%q", object.ProductReferenceID)
	}
	if object.ExternalAction != "2" {
		t.Fatalf("external_action=%q", object.ExternalAction)
	}
}

func TestCompileNextBindsPromotionToConfirmedProject(t *testing.T) {
	repo := stagedCreateRepoStub{version: stagedTestVersion(), mappings: map[string]PlatformEntityMapping{
		"project/draft_p1":   {InternalObjectKind: "project", InternalObjectID: "draft_p1", Status: PlatformEntityMappingConfirmed, PlatformObjectID: "7680332723195904041", BrowserRpaRunID: "run_1", PlatformObjectKind: "project"},
		"promotion/draft_r1": pendingMapping("promotion", "draft_r1", "run_1"),
	}}
	provider := BrowserRpaAuthorityProvider{Repository: repo}
	object, pending, err := provider.CompileNext(context.Background(), stagedTestRun())
	if err != nil || !pending {
		t.Fatalf("pending=%t err=%v", pending, err)
	}
	if object.Kind != "promotion" || object.DependsOnPlatformID != "7680332723195904041" {
		t.Fatalf("object=%#v", object)
	}
	if len(object.MaterialReferenceIDs) != 1 || object.MaterialReferenceIDs[0] != "v03033g10000d8kj9q2ljht6dsph9pm0" {
		t.Fatalf("materials=%v", object.MaterialReferenceIDs)
	}
}

func TestCompileNextStopsOnDailyBudgetContractGap(t *testing.T) {
	version := stagedTestVersion()
	version.PlatformConfiguration.Payload.OceanEngine.Project.BudgetAndBidding.BudgetMode = OceanEngineBudgetModeDaily
	repo := stagedCreateRepoStub{version: version, mappings: map[string]PlatformEntityMapping{
		"project/draft_p1": pendingMapping("project", "draft_p1", "run_1"),
	}}
	provider := BrowserRpaAuthorityProvider{Repository: repo}
	_, _, err := provider.CompileNext(context.Background(), stagedTestRun())
	if err == nil {
		t.Fatal("daily budget create must fail closed")
	}
}

func TestCompileNextReportsNothingPending(t *testing.T) {
	repo := stagedCreateRepoStub{version: stagedTestVersion(), mappings: map[string]PlatformEntityMapping{
		"project/draft_p1":   {InternalObjectKind: "project", InternalObjectID: "draft_p1", Status: PlatformEntityMappingConfirmed, PlatformObjectID: "1", BrowserRpaRunID: "run_1", PlatformObjectKind: "project"},
		"promotion/draft_r1": {InternalObjectKind: "promotion", InternalObjectID: "draft_r1", Status: PlatformEntityMappingConfirmed, PlatformObjectID: "2", BrowserRpaRunID: "run_1", PlatformObjectKind: "promotion"},
	}}
	provider := BrowserRpaAuthorityProvider{Repository: repo}
	_, pending, err := provider.CompileNext(context.Background(), stagedTestRun())
	if err != nil || pending {
		t.Fatalf("pending=%t err=%v", pending, err)
	}
}

func TestCompileNextFailsClosedWithoutRepository(t *testing.T) {
	provider := BrowserRpaAuthorityProvider{}
	_, _, err := provider.CompileNext(context.Background(), stagedTestRun())
	if !errors.Is(err, browserautomation.ErrInvalidContract) {
		t.Fatalf("err=%v", err)
	}
}

package delivery

import (
	"context"
	"github.com/shikanon/cookies/internal/platform/contract"
	"testing"
)

type fieldAccountResolver struct{ t *testing.T }

func (r fieldAccountResolver) ResolveExternalAccountID(_ context.Context, org, project, account string) (string, error) {
	if org != "org_a" || project != "project_a" || account != "stable-account" {
		r.t.Fatalf("wrong account resolution: %s/%s/%s", org, project, account)
	}
	return "456", nil
}

type fieldReaderStub struct {
	wrongIdentity bool
	calls         int
}

func (r *fieldReaderStub) ReadFieldCapabilities(_ context.Context, request FieldCapabilityRequest) (FieldCapabilitySnapshot, error) {
	r.calls++
	result := FieldCapabilitySnapshot{AccountID: request.AccountID, ObjectID: request.ObjectID, Kind: request.Kind, ObservedAt: "2026-09-10T08:00:00Z"}
	if r.wrongIdentity {
		result.ObjectID = "wrong"
	}
	return result, nil
}

func TestFieldInspectionUsesScopedStableAccountAndRejectsMismatchedEvidence(t *testing.T) {
	repo := newControlledMemoryRepository()
	configuration := &PlatformConfiguration{ConfigurationID: "config", Payload: PlatformConfigurationPayload{OceanEngine: &OceanEngineConfiguration{Project: &OceanEngineProjectDraft{ProjectDraftID: "draft", AccountReference: StableReference{ID: "stable-account"}, ProjectName: "project"}}}}
	version := DeliveryPlanVersion{PlatformConfiguration: configuration}
	repo.plans[repositoryKey("org_a", "project_a", "plan")] = DeliveryPlan{ID: "plan", CurrentVersion: version, Versions: []DeliveryPlanVersion{version}}
	repo.mappings[repositoryKey("org_a", "project_a", "map")] = PlatformEntityMapping{ID: "map", PlanID: "plan", ConfigurationID: "config", OrganizationID: "org_a", ProjectID: "project_a", AccountReferenceID: "456", InternalObjectKind: "project", InternalObjectID: "draft", PlatformObjectID: "123", Status: PlatformEntityMappingConfirmed}
	reader := &fieldReaderStub{}
	service := Service{Repository: repo, Projects: testProjects{}, ExternalAccountIDs: fieldAccountResolver{t}, FieldCapabilities: reader}
	actor := contract.ActorContext{OrganizationID: "org_a", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "operator"}, Scopes: contract.ScopesFromStrings([]string{string(ScopeRead)})}
	result, err := service.ReadObjectFieldCapabilities(context.Background(), actor, "project_a", "map")
	if err != nil || result.AccountID != "456" || len(result.Fields) == 0 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	reader.wrongIdentity = true
	if _, err := service.ReadObjectFieldCapabilities(context.Background(), actor, "project_a", "map"); err == nil {
		t.Fatal("accepted mismatched object")
	}
	actor.OrganizationID = "other"
	if _, err := service.ReadObjectFieldCapabilities(context.Background(), actor, "project_a", "map"); err == nil || reader.calls != 2 {
		t.Fatal("cross-organization inspection reached browser")
	}
}

package delivery

import (
	"testing"
)

func TestPlanObjectPreviewPreservesProjectAcrossVersionsAndFindsPromotionChanges(t *testing.T) {
	old := PlatformConfiguration{ConfigurationID: "configuration-1", Payload: PlatformConfigurationPayload{OceanEngine: &OceanEngineConfiguration{
		Project:    &OceanEngineProjectDraft{ProjectDraftID: "project-old-version", ProjectName: "project"},
		Promotions: []OceanEnginePromotionDraft{{PromotionDraftID: "unit-1", PromotionName: "unit", BudgetAndBidding: &OceanEngineBudgetAndBidding{DailyBudgetMinor: 30000}}},
	}}}
	next := old
	ocean := *old.Payload.OceanEngine
	project := *ocean.Project
	project.ProjectDraftID = "project-legacy-new-version"
	ocean.Project = &project
	ocean.Promotions = append([]OceanEnginePromotionDraft(nil), ocean.Promotions...)
	ocean.Promotions[0].BudgetAndBidding = &OceanEngineBudgetAndBidding{DailyBudgetMinor: 40000}
	ocean.Promotions = append(ocean.Promotions, OceanEnginePromotionDraft{PromotionDraftID: "unit-2", PromotionName: "new unit"})
	next.Payload.OceanEngine = &ocean
	next.ConfigurationID = "configuration-2"
	plan := DeliveryPlan{ID: "plan", Version: 2, CurrentVersion: DeliveryPlanVersion{PlatformConfiguration: &next}}
	mappings := []PlatformEntityMapping{
		{ID: "project-mapping", PlanID: "plan", ConfigurationID: old.ConfigurationID, InternalObjectKind: "project", InternalObjectID: "project-old-version", PlatformObjectID: "123", Status: PlatformEntityMappingConfirmed},
		{ID: "unit-mapping", PlanID: "plan", ConfigurationID: old.ConfigurationID, InternalObjectKind: "promotion", InternalObjectID: "unit-1", PlatformObjectID: "456", Status: PlatformEntityMappingConfirmed},
	}
	preview, err := buildPlanObjectPreview(plan, mappings, []DeliveryPlanVersion{{PlatformConfiguration: &old}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Objects) != 3 || preview.Objects[0].Action != "unchanged" || preview.Objects[0].PlatformID != "123" || preview.Objects[1].Action != "update" || preview.Objects[2].Action != "create" {
		t.Fatalf("preview=%+v", preview)
	}
	if len(preview.Objects[1].ChangedFields) != 1 || preview.Objects[1].ChangedFields[0] != "budget_and_bidding" {
		t.Fatalf("budget diff=%+v", preview.Objects[1])
	}
	preview, err = buildPlanObjectPreview(plan, mappings, []DeliveryPlanVersion{{PlatformConfiguration: &old}}, map[string]int64{"unit-mapping": 40000})
	if err != nil || preview.Objects[1].Action != "unchanged" {
		t.Fatalf("confirmed budget update was planned again: %+v %v", preview, err)
	}
	project.ProjectName = "renamed"
	preview, err = buildPlanObjectPreview(plan, mappings, []DeliveryPlanVersion{{PlatformConfiguration: &old}}, nil)
	if err != nil || preview.Objects[0].Action != "update" {
		t.Fatalf("renamed project must not be recreated: %+v %v", preview, err)
	}
	preview, err = buildPlanObjectPreview(plan, mappings[1:], []DeliveryPlanVersion{{PlatformConfiguration: &old}}, nil)
	if err != nil || preview.Objects[0].Action != "blocked" {
		t.Fatalf("missing parent binding created a replacement project: %+v %v", preview, err)
	}
	mappings[0].Status = PlatformEntityMappingPending
	preview, err = buildPlanObjectPreview(plan, mappings, []DeliveryPlanVersion{{PlatformConfiguration: &old}}, nil)
	if err != nil || preview.Objects[0].Action != "blocked" {
		t.Fatalf("pending binding must not be recreated: %+v %v", preview, err)
	}
	mappings = append(mappings, mappings[0])
	if _, err := buildPlanObjectPreview(plan, mappings, nil, nil); err == nil {
		t.Fatal("ambiguous project binding accepted")
	}
}

func TestBoundObjectsRejectIdentityChangesAndUncalibratedFields(t *testing.T) {
	previous := &OceanEngineConfiguration{Project: &OceanEngineProjectDraft{ProjectDraftID: "project", ProjectName: "original"}, Promotions: []OceanEnginePromotionDraft{{PromotionDraftID: "unit", PromotionName: "unit", BudgetAndBidding: &OceanEngineBudgetAndBidding{DailyBudgetMinor: 30000, ChargingMode: "CPC"}}}}
	preview := PlanObjectPreview{Objects: []PlanObjectAction{{Kind: "project", InternalID: "project", MappingID: "project-map", PlatformID: "100"}, {Kind: "promotion", InternalID: "unit", MappingID: "unit-map", PlatformID: "200"}}}
	next := *previous
	next.Promotions = append([]OceanEnginePromotionDraft(nil), previous.Promotions...)
	budget := *previous.Promotions[0].BudgetAndBidding
	budget.DailyBudgetMinor = 40000
	next.Promotions[0].BudgetAndBidding = &budget
	if err := validateBoundObjectEdits(previous, &next, preview); err != nil {
		t.Fatalf("daily budget target rejected: %v", err)
	}
	budget.ChargingMode = "CPM"
	if err := validateBoundObjectEdits(previous, &next, preview); err == nil {
		t.Fatal("charging mode update accepted")
	}
	next.Promotions[0] = previous.Promotions[0]
	next.Promotions[0].PromotionDraftID = "replacement"
	if err := validateBoundObjectEdits(previous, &next, preview); err == nil {
		t.Fatal("bound unit identity replacement accepted")
	}
	next.Promotions = previous.Promotions
	project := *previous.Project
	project.ProjectName = "unverified edit"
	next.Project = &project
	if err := validateBoundObjectEdits(previous, &next, preview); err == nil {
		t.Fatal("uncalibrated project edit accepted")
	}
}

func TestFieldPoliciesDoNotInferPlatformRestrictionsFromCookiesSupport(t *testing.T) {
	for _, bound := range []bool{false, true} {
		fields := objectFieldPolicies("project", bound, &OceanEngineProjectDraft{ProjectName: "project"})
		for _, field := range fields {
			expected := "unknown"
			if !bound {
				expected = "not_applicable"
			}
			if field.Platform.State != expected || field.Cookies.State != field.State {
				t.Fatalf("platform and Cookies policies mixed: %+v", field)
			}
		}
	}
}

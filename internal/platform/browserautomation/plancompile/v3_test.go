package plancompile

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

type v3SourceStub struct {
	version  delivery.DeliveryPlanVersion
	mappings []delivery.PlatformEntityMapping
}

type v3AccountResolverStub struct {
	externalID string
}

type v3PlatformObjectSourceStub struct {
	objects []connector.PlatformObject
	query   connector.PlatformObjectQuery
}

func (s *v3PlatformObjectSourceStub) ListPlatformObjects(_ context.Context, query connector.PlatformObjectQuery) ([]connector.PlatformObject, error) {
	s.query = query
	return s.objects, nil
}

func (s v3AccountResolverStub) ResolveExternalAccountID(context.Context, string, string, string) (string, error) {
	return s.externalID, nil
}

func (s v3SourceStub) GetPlanVersion(context.Context, contract.OrganizationID, contract.ProjectID, string, int) (delivery.DeliveryPlanVersion, error) {
	return s.version, nil
}

func (s v3SourceStub) ListPlatformEntityMappings(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]delivery.PlatformEntityMapping, error) {
	return s.mappings, nil
}

func TestV3CompilerHydratesStaleProductImageIdentityFromCollector(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, _ := executableConfigurationFixture(now)
	reference := &configuration.Payload.OceanEngine.Promotions[0].ProductImageReferences[0]
	reference.Scope = "account:account_internal"
	reference.AuditAttributes["image_src_identity"] = "api/connector/v1/projects/project-1/platform-objects/image-1/preview"
	reference.AuditAttributes["connector_platform_object_id"] = "connector-image-1"
	source := &v3PlatformObjectSourceStub{objects: []connector.PlatformObject{{
		ID: "connector-image-1", Kind: connector.PlatformObjectProductImage, PlatformObjectID: reference.ID,
		Metadata: map[string]any{"web_uri": "tos-cn-i-example/stable-image"},
	}}}
	compiler := V3Compiler{PlatformObjects: source}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1"}

	hydrated, err := compiler.hydrateProductImageEvidence(context.Background(), run, configuration)
	if err != nil {
		t.Fatal(err)
	}
	identity := hydrated.Payload.OceanEngine.Promotions[0].ProductImageReferences[0].AuditAttributes["image_src_identity"]
	if identity != "tos-cn-i-example/stable-image" {
		t.Fatalf("hydrated identity = %q", identity)
	}
	if configuration.Payload.OceanEngine.Promotions[0].ProductImageReferences[0].AuditAttributes["image_src_identity"] == identity {
		t.Fatal("hydration mutated the immutable configuration")
	}
	if source.query.AccountID != "account_internal" || source.query.Kind != connector.PlatformObjectProductImage || source.query.Search != reference.ID {
		t.Fatalf("Collector query = %#v", source.query)
	}
}

func TestV3CompilerHydratesGenericOptimizationTargetFromCollector(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, _ := executableConfigurationFixture(now)
	reference := configuration.Payload.OceanEngine.Project.OptimizationTargetReference
	reference.ID = "-1"
	reference.Scope = "account:account_internal"
	reference.SemanticKey = "external_action:-1"
	reference.DisplayNameSnapshot = ""
	source := &v3PlatformObjectSourceStub{objects: []connector.PlatformObject{{
		Kind: connector.PlatformObjectOptimizationTarget, PlatformObjectID: "-1", DisplayName: "展示量",
	}}}
	compiler := V3Compiler{PlatformObjects: source}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1"}

	hydrated, err := compiler.hydrateOptimizationTargetEvidence(context.Background(), run, configuration)
	if err != nil {
		t.Fatal(err)
	}
	hydratedReference := hydrated.Payload.OceanEngine.Project.OptimizationTargetReference
	if hydratedReference.SemanticKey != "impression" || hydratedReference.DisplayNameSnapshot != "展示量" {
		t.Fatalf("hydrated reference = %#v", hydratedReference)
	}
	if reference.SemanticKey != "external_action:-1" || reference.DisplayNameSnapshot != "" {
		t.Fatal("hydration mutated the immutable configuration")
	}
	if source.query.AccountID != "account_internal" || source.query.Kind != connector.PlatformObjectOptimizationTarget || source.query.Search != "-1" {
		t.Fatalf("Collector query = %#v", source.query)
	}
}

func TestParentContextInfersKnownOptimizationTargetFromDisplayName(t *testing.T) {
	project := delivery.OceanEngineProjectDraft{
		MarketingPurpose: "lead_generation", DeliveryMode: "ubmax", PlacementStrategy: "preferred_media",
		OptimizationTargetReference: &delivery.StableReference{
			ID: "-1", DisplayNameSnapshot: "展示量",
			AuditAttributes: map[string]string{"capability_snapshot_id": "snapshot-1", "capability_context_hash": strings.Repeat("a", 64)},
		},
	}
	parent, err := parentContext(project)
	if err != nil {
		t.Fatal(err)
	}
	if parent.OptimizationTarget != "impression" || parent.OptimizationTargetExternalAction != "-1" {
		t.Fatalf("parent context = %#v", parent)
	}
}

func TestV3CompilerConvertsBoundBudgetRunAndIssuesOneTimeAuthority(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configurationHash := strings.Repeat("b", 64)
	planHash := strings.Repeat("a", 64)
	optimization := delivery.StableReference{ID: "in_app_order"}
	account := delivery.StableReference{ID: "1855554434276391", State: delivery.ReferenceResolved}
	configuration := &delivery.PlatformConfiguration{
		CanonicalHash: configurationHash,
		Payload: delivery.PlatformConfigurationPayload{OceanEngine: &delivery.OceanEngineConfiguration{Project: &delivery.OceanEngineProjectDraft{
			AccountReference: account,
			Carrier:          "owned_landing_page", OptimizationTargetReference: &optimization, DeepOptimizationMode: "conversion_roi",
			DeliveryMode: "manual", PlacementStrategy: "automatic",
		}}},
	}
	compiler := V3Compiler{Source: v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, PlatformConfiguration: configuration}}, Now: func() time.Time { return now }}
	run := browserautomation.BrowserRpaRun{
		OrganizationID: "org_1", ProjectID: "project_1", AccountID: "1855554434276391",
		Authority: browserautomation.AuthorityBinding{
			Action: "update_promotion_budget", PlanID: "plan_1", PlanVersion: 2, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configurationHash,
			ParentPlatformProjectID: "7677595885572784182", TargetPlatformObjectID: "7683558668450021382", BudgetLimitMinor: 30000,
			PromotionMutation: &browserautomation.PromotionMutationBinding{CurrentDailyBudgetMinor: 40000, TargetDailyBudgetMinor: 30000},
		},
	}
	policy := browserautomation.SitePolicy{
		AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"},
		AllowedPageKinds: []string{"promotion_edit"}, AllowedPlatformProjects: []string{"7677595885572784182"},
	}
	prepare, err := compiler.CompilePrepareV3(context.Background(), run, policy)
	if err != nil {
		t.Fatalf("compile prepare: %v", err)
	}
	var preparePlan map[string]any
	if err := json.Unmarshal(prepare, &preparePlan); err != nil {
		t.Fatal(err)
	}
	if preparePlan["schema_version"] != "oceanengine-playwright-rpa-plan/v3" || preparePlan["plan_kind"] != "promotion_edit" || preparePlan["allow_remote_write"] != false {
		t.Fatalf("prepare plan = %#v", preparePlan)
	}

	attempt := browserautomation.ControlledActionAttempt{ID: "attempt_1", Status: browserautomation.ControlledActionAuthorized, CreatedAt: now}
	submit, err := compiler.CompileSubmitV3(context.Background(), run, attempt, policy, "single-use-token")
	if err != nil {
		t.Fatalf("compile submit: %v", err)
	}
	var submitPlan map[string]any
	if err := json.Unmarshal(submit, &submitPlan); err != nil {
		t.Fatal(err)
	}
	authority, ok := submitPlan["execution_authority"].(map[string]any)
	if !ok || authority["authority_id"] != "attempt_1" || authority["confirm_token_sha256"] == "single-use-token" {
		t.Fatalf("execution authority = %#v", authority)
	}
	delete(submitPlan, "execution_authority")
	expectedHash, err := contract.CanonicalJSONHash(submitPlan)
	if err != nil {
		t.Fatal(err)
	}
	if authority["plan_sha256"] != expectedHash || submitPlan["allow_remote_write"] != true || submitPlan["maximum_final_clicks"] != float64(1) {
		t.Fatalf("submit authority/plan mismatch: authority=%#v plan=%#v", authority, submitPlan)
	}
}

func TestV3CompilerFailsClosedForUnsupportedControlActions(t *testing.T) {
	compiler := V3Compiler{Source: v3SourceStub{}}
	run := browserautomation.BrowserRpaRun{Authority: browserautomation.AuthorityBinding{Action: "pause_promotion", PlanID: "plan_1", PlanVersion: 1}}
	_, err := compiler.CompilePrepareV3(context.Background(), run, browserautomation.SitePolicy{})
	if err == nil || !strings.Contains(err.Error(), "no Runner v3 one-form path") {
		t.Fatalf("unsupported action error = %v", err)
	}
}

func TestV3CompilerRunsOnePromotionCreateFromBoundConfiguration(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	planHash := strings.Repeat("a", 64)
	compiler := V3Compiler{Source: v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}}, Now: func() time.Time { return now }}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1", AccountID: "1855554434276391", Authority: browserautomation.AuthorityBinding{Action: "create_promotions_in_existing_project", PlanID: "plan_1", PlanVersion: 1, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configuration.CanonicalHash, ParentPlatformProjectID: "7677595885572784182", BudgetLimitMinor: 30000}}
	policy := browserautomation.SitePolicy{AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"promotion_create"}, AllowedPlatformProjects: []string{"7677595885572784182"}}
	raw, err := compiler.CompilePrepareV3(context.Background(), run, policy)
	if err != nil {
		t.Fatal(err)
	}
	var plan map[string]any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if plan["plan_kind"] != "promotion_create" || plan["parent_project_reference"] != "7677595885572784182" || plan["status"] != "ready" {
		t.Fatalf("plan = %#v", plan)
	}
	attempt := browserautomation.ControlledActionAttempt{ID: "attempt_1", Status: browserautomation.ControlledActionAuthorized, CreatedAt: now}
	submit, err := compiler.CompileSubmitV3(context.Background(), run, attempt, policy, "token")
	if err != nil {
		t.Fatal(err)
	}
	var submitted map[string]any
	if err := json.Unmarshal(submit, &submitted); err != nil {
		t.Fatal(err)
	}
	authority := submitted["execution_authority"].(map[string]any)
	if authority["schedule_date"] != configuration.Payload.OceanEngine.Project.Schedule.StartAt.Format(time.DateOnly) {
		t.Fatalf("schedule authority = %#v", authority)
	}
}

func TestV3CompilerSelectsProjectAsFirstStagedCreateForm(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	planHash := strings.Repeat("a", 64)
	compiler := V3Compiler{Source: v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}}, Now: func() time.Time { return now }}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1", AccountID: "1855554434276391", Authority: browserautomation.AuthorityBinding{Action: "create_project_and_promotions", PlanID: "plan_1", PlanVersion: 1, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configuration.CanonicalHash}}
	raw, err := compiler.CompilePrepareV3(context.Background(), run, browserautomation.SitePolicy{AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}})
	if err != nil {
		t.Fatal(err)
	}
	var plan v3Plan
	if json.Unmarshal(raw, &plan) != nil || plan.PlanKind != "project_create" || plan.InternalObjectKind != "project" || plan.InternalObjectID != "project-draft-1" {
		t.Fatalf("staged project plan = %#v", plan)
	}
	if plan.ParentContext.OptimizationTargetExternalAction != "9001" {
		t.Fatalf("external_action evidence = %#v", plan.ParentContext)
	}
}

func TestV3CompilerResolvesConnectorAccountBeforeBuildingRunnerPlan(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Project.AccountReference.ID = "oeacct_stable"
	planHash := strings.Repeat("a", 64)
	externalID := "1855554434276391"
	compiler := V3Compiler{
		Source:          v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}},
		AccountResolver: v3AccountResolverStub{externalID: externalID},
		Now:             func() time.Time { return now },
	}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1", AccountID: externalID, Authority: browserautomation.AuthorityBinding{Action: "create_project_and_promotions", PlanID: "plan_1", PlanVersion: 1, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configuration.CanonicalHash}}
	raw, err := compiler.CompilePrepareV3(context.Background(), run, browserautomation.SitePolicy{AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}})
	if err != nil {
		t.Fatal(err)
	}
	var plan v3Plan
	if err := json.Unmarshal(raw, &plan); err != nil || plan.AccountReference != externalID {
		t.Fatalf("resolved account plan = %#v err=%v", plan, err)
	}
}

func TestV3CompilerAdvancesThroughMappedProjectAndPromotions(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	second := configuration.Payload.OceanEngine.Promotions[0]
	second.PromotionDraftID = "promotion-draft-2"
	second.PromotionName = "第二个测试单元"
	configuration.Payload.OceanEngine.Promotions = append(configuration.Payload.OceanEngine.Promotions, second)
	planHash := strings.Repeat("a", 64)
	projectMapping := delivery.PlatformEntityMapping{ID: "mapping-project", ConfigurationID: configuration.ConfigurationID, AccountReferenceID: "1855554434276391", InternalObjectKind: "project", InternalObjectID: "project-draft-1", PlatformObjectKind: "project", PlatformObjectID: "7677595885572784182", Status: delivery.PlatformEntityMappingConfirmed}
	firstPromotionMapping := delivery.PlatformEntityMapping{ID: "mapping-promotion-1", ConfigurationID: configuration.ConfigurationID, AccountReferenceID: "1855554434276391", InternalObjectKind: "promotion", InternalObjectID: "promotion-draft-1", PlatformObjectKind: "promotion", PlatformObjectID: "7683558668450021382", Status: delivery.PlatformEntityMappingConfirmed}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1", AccountID: "1855554434276391", Authority: browserautomation.AuthorityBinding{Action: "create_project_and_promotions", PlanID: "plan_1", PlanVersion: 1, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configuration.CanonicalHash}}
	policy := browserautomation.SitePolicy{AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create", "promotion_create"}}

	compiler := V3Compiler{Source: v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}, mappings: []delivery.PlatformEntityMapping{projectMapping}}, Now: func() time.Time { return now }}
	raw, err := compiler.CompilePrepareV3(context.Background(), run, policy)
	if err != nil {
		t.Fatal(err)
	}
	var first v3Plan
	_ = json.Unmarshal(raw, &first)
	if first.PlanKind != "promotion_create" || first.InternalObjectID != "promotion-draft-1" || first.ParentProjectReference != projectMapping.PlatformObjectID {
		t.Fatalf("first promotion plan = %#v", first)
	}

	compiler.Source = v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}, mappings: []delivery.PlatformEntityMapping{projectMapping, firstPromotionMapping}}
	raw, err = compiler.CompilePrepareV3(context.Background(), run, policy)
	if err != nil {
		t.Fatal(err)
	}
	var next v3Plan
	_ = json.Unmarshal(raw, &next)
	if next.PlanKind != "promotion_create" || next.InternalObjectID != "promotion-draft-2" || next.ParentProjectReference != projectMapping.PlatformObjectID {
		t.Fatalf("second promotion plan = %#v", next)
	}
}

func TestV3CompilerReportsUnavailableObjectsBeforeProjectCreate(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	product := configuration.Payload.OceanEngine.Project.MarketingProductReference
	product.Namespace = "cookies"
	product.ID = "product_internal_1"
	product.AuditAttributes = nil
	intent.Payload.ProductReferences[0] = *product
	planHash := strings.Repeat("a", 64)
	compiler := V3Compiler{Source: v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}}, Now: func() time.Time { return now }}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1", AccountID: "1855554434276391", Authority: browserautomation.AuthorityBinding{Action: "create_project_and_promotions", PlanID: "plan_1", PlanVersion: 1, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configuration.CanonicalHash}}
	policy := browserautomation.SitePolicy{AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}}

	raw, err := compiler.CompilePrepareV3(context.Background(), run, policy)
	if err != nil {
		t.Fatal(err)
	}
	var plan v3Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Status != "blocked" || !slices.Contains(plan.BlockedReasons, unavailablePlatformObjectsReason) {
		t.Fatalf("blocked plan = %#v", plan)
	}
	missing := slices.IndexFunc(plan.ObjectAvailability, func(value V3ObjectAvailability) bool {
		return value.FieldKey == "project.marketing_product_reference" && !value.Available
	})
	if missing < 0 || plan.ObjectAvailability[missing].InternalObjectID != "product_internal_1" {
		t.Fatalf("object availability = %#v", plan.ObjectAvailability)
	}
	attempt := browserautomation.ControlledActionAttempt{ID: "attempt_1", Status: browserautomation.ControlledActionAuthorized, CreatedAt: now}
	if _, err := compiler.CompileSubmitV3(context.Background(), run, attempt, policy, "token"); err == nil || !strings.Contains(err.Error(), unavailablePlatformObjectsReason) {
		t.Fatalf("submit error = %v", err)
	}
}

func TestV3CompilerReturnsBlockedPlanForIncompletePromotionConfiguration(t *testing.T) {
	now := time.Date(2026, 8, 25, 6, 0, 0, 0, time.UTC)
	configuration, intent := executableConfigurationFixture(now)
	configuration.Payload.OceanEngine.Promotions[0].PromotionName = ""
	planHash := strings.Repeat("a", 64)
	compiler := V3Compiler{Source: v3SourceStub{version: delivery.DeliveryPlanVersion{CanonicalHash: planHash, DeliveryIntent: &intent, PlatformConfiguration: &configuration}}, Now: func() time.Time { return now }}
	run := browserautomation.BrowserRpaRun{OrganizationID: "org_1", ProjectID: "project_1", AccountID: "1855554434276391", Authority: browserautomation.AuthorityBinding{Action: "create_project_and_promotions", PlanID: "plan_1", PlanVersion: 1, PlanCanonicalHash: planHash, ConfigurationCanonicalHash: configuration.CanonicalHash}}
	policy := browserautomation.SitePolicy{AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create"}}

	raw, err := compiler.CompilePrepareV3(context.Background(), run, policy)
	if err != nil {
		t.Fatal(err)
	}
	var plan v3Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Status != "blocked" || !slices.Contains(plan.BlockedReasons, invalidDeliveryConfigurationReason) || len(plan.ConfigurationIssues) != 1 {
		t.Fatalf("blocked plan = %#v", plan)
	}
	if !strings.Contains(plan.ConfigurationIssues[0], "requires copy, source, and name") || plan.AllowRemoteWrite || plan.MaximumFinalClicks != 0 {
		t.Fatalf("configuration issues = %#v", plan.ConfigurationIssues)
	}
}

package plancompile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/platform/browserautomation/rparunner"
	"github.com/shikanon/cookies/internal/platform/connector"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/oceanengineconstraints"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

const (
	v3ParentManifestID                 = "oceanengine-ecommerce-parent-condition-2026-08-24"
	v3PromotionEdit                    = "promotion_edit"
	invalidDeliveryConfigurationReason = "DELIVERY_CONFIGURATION_INVALID"
)

type V3ConfigurationSource interface {
	GetPlanVersion(context.Context, contract.OrganizationID, contract.ProjectID, string, int) (delivery.DeliveryPlanVersion, error)
}

type V3MappingSource interface {
	ListPlatformEntityMappings(context.Context, contract.OrganizationID, contract.ProjectID, string) ([]delivery.PlatformEntityMapping, error)
}

type V3AccountResolver interface {
	ResolveExternalAccountID(context.Context, string, string, string) (string, error)
}

type V3PlatformObjectSource interface {
	ListPlatformObjects(context.Context, connector.PlatformObjectQuery) ([]connector.PlatformObject, error)
}

// V3Compiler converts one immutable Cookies run into one Runner v3 form plan.
// The first controlled path is promotion budget edit. Other actions fail
// closed until Runner v3 has an equivalent one-form contract.
type V3Compiler struct {
	Source          V3ConfigurationSource
	AccountResolver V3AccountResolver
	PlatformObjects V3PlatformObjectSource
	Now             func() time.Time
}

var _ rparunner.V3PlanCompiler = V3Compiler{}

type v3ParentContext struct {
	Carrier                          string            `json:"carrier"`
	OptimizationTarget               string            `json:"optimization_target"`
	OptimizationTargetExternalAction string            `json:"optimization_target_external_action,omitempty"`
	DeepOptimization                 string            `json:"deep_optimization"`
	DeliveryMode                     string            `json:"delivery_mode"`
	PlacementMode                    string            `json:"placement_mode"`
	SearchTargetingExpansion         bool              `json:"search_targeting_expansion,omitempty"`
	ParentReferences                 map[string]string `json:"parent_references,omitempty"`
}

type v3Step struct {
	ID              string                                `json:"id"`
	Kind            string                                `json:"kind"`
	PageKind        string                                `json:"page_kind"`
	FieldKey        string                                `json:"field_key,omitempty"`
	Operation       string                                `json:"operation,omitempty"`
	Scope           string                                `json:"scope,omitempty"`
	Target          string                                `json:"target,omitempty"`
	Value           any                                   `json:"value,omitempty"`
	ValueState      string                                `json:"value_state,omitempty"`
	Required        *bool                                 `json:"required,omitempty"`
	RemoteWrite     bool                                  `json:"remote_write"`
	Blocked         bool                                  `json:"blocked"`
	BlockReason     string                                `json:"block_reason,omitempty"`
	MoneyConstraint *oceanengineconstraints.BidConstraint `json:"money_constraint,omitempty"`
}

type v3ExecutionAuthority struct {
	SchemaVersion      string `json:"schema_version"`
	AuthorityID        string `json:"authority_id"`
	PlanSHA256         string `json:"plan_sha256"`
	ConfirmTokenSHA256 string `json:"confirm_token_sha256"`
	IssuedAt           string `json:"issued_at"`
	ExpiresAt          string `json:"expires_at"`
	AccountReference   string `json:"account_reference"`
	PermittedPlanKind  string `json:"permitted_plan_kind"`
	MaximumMoneyCNY    int64  `json:"maximum_money_cny"`
	ScheduleDate       string `json:"schedule_date"`
	MaximumFinalClicks int    `json:"maximum_final_clicks"`
}

type v3Plan struct {
	SchemaVersion             string                 `json:"schema_version"`
	PlanKind                  string                 `json:"plan_kind"`
	Browser                   string                 `json:"browser"`
	Mode                      string                 `json:"mode"`
	Status                    string                 `json:"status"`
	AccountReference          string                 `json:"account_reference"`
	ObjectReference           string                 `json:"object_reference,omitempty"`
	ParentProjectReference    string                 `json:"parent_project_reference,omitempty"`
	InternalObjectKind        string                 `json:"internal_object_kind,omitempty"`
	InternalObjectID          string                 `json:"internal_object_id,omitempty"`
	ParentConditionManifestID string                 `json:"parent_condition_manifest_id"`
	ParentContext             v3ParentContext        `json:"parent_context"`
	BlockedReasons            []string               `json:"blocked_reasons"`
	ConfigurationIssues       []string               `json:"configuration_issues,omitempty"`
	ObjectAvailability        []V3ObjectAvailability `json:"object_availability,omitempty"`
	ExecutionAuthority        *v3ExecutionAuthority  `json:"execution_authority,omitempty"`
	Steps                     []v3Step               `json:"steps"`
	AllowRemoteWrite          bool                   `json:"allow_remote_write"`
	MaximumFinalClicks        int                    `json:"maximum_final_clicks"`
}

func (c V3Compiler) CompilePrepareV3(ctx context.Context, run browserautomation.BrowserRpaRun, policy browserautomation.SitePolicy) (json.RawMessage, error) {
	plan, err := c.preparePlan(ctx, run, policy)
	if err != nil {
		return nil, err
	}
	return json.Marshal(plan)
}

func (c V3Compiler) CompileSubmitV3(ctx context.Context, run browserautomation.BrowserRpaRun, attempt browserautomation.ControlledActionAttempt, policy browserautomation.SitePolicy, confirmToken string) (json.RawMessage, error) {
	if strings.TrimSpace(confirmToken) == "" || attempt.ID == "" || attempt.Status != browserautomation.ControlledActionAuthorized {
		return nil, fmt.Errorf("one-time execution authority is required")
	}
	plan, err := c.preparePlan(ctx, run, policy)
	if err != nil {
		return nil, err
	}
	if len(plan.BlockedReasons) != 0 {
		return nil, fmt.Errorf("Runner v3 plan is blocked: %s", strings.Join(plan.BlockedReasons, ","))
	}
	plan.Mode = "submit"
	plan.AllowRemoteWrite = true
	plan.MaximumFinalClicks = 1
	boundary := &plan.Steps[len(plan.Steps)-1]
	boundary.Blocked = false
	boundary.BlockReason = ""

	planHash, err := contract.CanonicalJSONHash(plan)
	if err != nil {
		return nil, fmt.Errorf("hash Runner v3 plan: %w", err)
	}
	issuedAt := attempt.CreatedAt.UTC()
	if issuedAt.IsZero() {
		issuedAt = c.now().UTC()
	}
	tokenDigest := sha256.Sum256([]byte(confirmToken))
	scheduleDate := issuedAt.In(time.FixedZone("CST", 8*60*60)).Format(time.DateOnly)
	if run.Authority.Action == "create_project_and_promotions" || run.Authority.Action == "create_promotions_in_existing_project" {
		version, loadErr := c.Source.GetPlanVersion(ctx, run.OrganizationID, run.ProjectID, run.Authority.PlanID, run.Authority.PlanVersion)
		if loadErr != nil || version.PlatformConfiguration == nil || version.PlatformConfiguration.Payload.OceanEngine == nil || version.PlatformConfiguration.Payload.OceanEngine.Project == nil {
			return nil, fmt.Errorf("load schedule for execution authority")
		}
		scheduleDate = version.PlatformConfiguration.Payload.OceanEngine.Project.Schedule.StartAt.In(time.FixedZone("CST", 8*60*60)).Format(time.DateOnly)
	}
	plan.ExecutionAuthority = &v3ExecutionAuthority{
		SchemaVersion: "browser-rpa-execution-authority/v1", AuthorityID: attempt.ID,
		PlanSHA256: planHash, ConfirmTokenSHA256: hex.EncodeToString(tokenDigest[:]),
		IssuedAt: issuedAt.Format(time.RFC3339Nano), ExpiresAt: issuedAt.Add(10 * time.Minute).Format(time.RFC3339Nano),
		AccountReference: plan.AccountReference, PermittedPlanKind: plan.PlanKind,
		MaximumMoneyCNY: run.Authority.BudgetLimitMinor / 100,
		ScheduleDate:    scheduleDate, MaximumFinalClicks: 1,
	}
	return json.Marshal(plan)
}

func (c V3Compiler) preparePlan(ctx context.Context, run browserautomation.BrowserRpaRun, policy browserautomation.SitePolicy) (v3Plan, error) {
	if c.Source == nil || run.Authority.PlanID == "" || run.Authority.PlanVersion < 1 {
		return v3Plan{}, fmt.Errorf("run has no immutable delivery plan binding")
	}
	if !slices.Contains([]string{"create_project_and_promotions", "create_promotions_in_existing_project", "update_promotion_budget"}, run.Authority.Action) {
		return v3Plan{}, fmt.Errorf("action %q has no Runner v3 one-form path", run.Authority.Action)
	}
	version, err := c.Source.GetPlanVersion(ctx, run.OrganizationID, run.ProjectID, run.Authority.PlanID, run.Authority.PlanVersion)
	if err != nil {
		return v3Plan{}, fmt.Errorf("load bound delivery plan: %w", err)
	}
	if version.CanonicalHash != run.Authority.PlanCanonicalHash || version.PlatformConfiguration == nil || version.PlatformConfiguration.CanonicalHash != run.Authority.ConfigurationCanonicalHash {
		return v3Plan{}, fmt.Errorf("bound delivery configuration changed")
	}
	configuration := version.PlatformConfiguration.Payload.OceanEngine
	if configuration == nil || configuration.Project == nil {
		return v3Plan{}, fmt.Errorf("bound delivery configuration has no OceanEngine project")
	}
	if configuration.Project.AccountReference.State != delivery.ReferenceResolved {
		return v3Plan{}, fmt.Errorf("configuration account path does not match the authorized account")
	}
	compiledConfiguration := *version.PlatformConfiguration
	compiledConfiguration, err = c.hydrateProductImageEvidence(ctx, run, compiledConfiguration)
	if err != nil {
		return v3Plan{}, err
	}
	compiledConfiguration, err = c.hydrateOptimizationTargetEvidence(ctx, run, compiledConfiguration)
	if err != nil {
		return v3Plan{}, err
	}
	configuration = compiledConfiguration.Payload.OceanEngine
	if configuration.Project.AccountReference.ID != run.AccountID {
		if c.AccountResolver == nil {
			return v3Plan{}, fmt.Errorf("configuration account path does not match the authorized account")
		}
		resolved, resolveErr := c.AccountResolver.ResolveExternalAccountID(ctx, string(run.OrganizationID), string(run.ProjectID), configuration.Project.AccountReference.ID)
		if resolveErr != nil || resolved != run.AccountID {
			return v3Plan{}, fmt.Errorf("configuration account path does not match the authorized account")
		}
		oceanCopy := *configuration
		projectCopy := *configuration.Project
		projectCopy.AccountReference.ID = run.AccountID
		oceanCopy.Project = &projectCopy
		compiledConfiguration.Payload.OceanEngine = &oceanCopy
		configuration = &oceanCopy
	}
	if len(policy.AllowedHosts) == 0 || len(policy.AllowedProtocols) == 0 {
		return v3Plan{}, fmt.Errorf("site policy has no allowed OceanEngine origin")
	}
	switch run.Authority.Action {
	case "create_promotions_in_existing_project":
		if !numericReference(run.AccountID) || !numericReference(run.Authority.ParentPlatformProjectID) || !slices.Contains(policy.AllowedPageKinds, "promotion_create") || !slices.Contains(policy.AllowedPlatformProjects, run.Authority.ParentPlatformProjectID) {
			return v3Plan{}, fmt.Errorf("site policy does not allow the bound promotion create form")
		}
		bindings, bindingErr := c.stagedBindings(ctx, run, compiledConfiguration)
		if bindingErr != nil {
			return v3Plan{}, bindingErr
		}
		bindings.ProjectPlatformID = run.Authority.ParentPlatformProjectID
		set, compileErr := CompileConfigurationV3(compiledConfiguration, version.DeliveryIntent, run.AccountID, bindings, c.now())
		if compileErr != nil {
			return blockedConfigurationPlan(run, "promotion_create", run.Authority.ParentPlatformProjectID, nil, compileErr), nil
		}
		form, formErr := nextUnboundPromotionForm(set.Forms)
		if formErr != nil {
			return v3Plan{}, formErr
		}
		return decodePlannedForm(form)
	case "create_project_and_promotions":
		availability := configurationObjectAvailability(*configuration)
		if slices.ContainsFunc(availability, func(value V3ObjectAvailability) bool { return !value.Available }) {
			required := true
			return v3Plan{
				SchemaVersion: rparunner.PlanSchemaV3, PlanKind: "project_create", Browser: "msedge", Mode: "prepare", Status: "blocked",
				AccountReference: run.AccountID, ParentConditionManifestID: v3ParentManifestID,
				BlockedReasons: []string{unavailablePlatformObjectsReason}, ObjectAvailability: availability,
				Steps:            []v3Step{{ID: "001-object-availability", Kind: "preflight", PageKind: "project_create", Required: &required, Blocked: true, BlockReason: unavailablePlatformObjectsReason}},
				AllowRemoteWrite: false, MaximumFinalClicks: 0,
			}, nil
		}
		bindings, bindingErr := c.stagedBindings(ctx, run, compiledConfiguration)
		if bindingErr != nil {
			return v3Plan{}, bindingErr
		}
		set, compileErr := CompileConfigurationV3(compiledConfiguration, version.DeliveryIntent, run.AccountID, bindings, c.now())
		if compileErr != nil {
			return blockedConfigurationPlan(run, "project_create", "", availability, compileErr), nil
		}
		form, formErr := nextStagedCreateForm(set.Forms)
		if formErr != nil {
			return v3Plan{}, formErr
		}
		plan, decodeErr := decodePlannedForm(form)
		if decodeErr != nil {
			return v3Plan{}, decodeErr
		}
		if !slices.Contains(policy.AllowedPageKinds, plan.PlanKind) {
			return v3Plan{}, fmt.Errorf("site policy does not allow the %s form", plan.PlanKind)
		}
		plan.ObjectAvailability = availability
		return plan, nil
	case "update_promotion_budget":
		// Continue with the exact one-field edit contract below.
	default:
		return v3Plan{}, fmt.Errorf("action %q has no Runner v3 one-form path", run.Authority.Action)
	}
	if run.Authority.PromotionMutation == nil {
		return v3Plan{}, fmt.Errorf("promotion budget action has no mutation binding")
	}
	if !numericReference(run.AccountID) || !numericReference(run.Authority.TargetPlatformObjectID) || !numericReference(run.Authority.ParentPlatformProjectID) {
		return v3Plan{}, fmt.Errorf("Runner v3 needs exact numeric account, promotion, and parent project references")
	}
	if !slices.Contains(policy.AllowedPageKinds, v3PromotionEdit) || !slices.Contains(policy.AllowedPlatformProjects, run.Authority.ParentPlatformProjectID) {
		return v3Plan{}, fmt.Errorf("site policy does not allow the promotion edit form")
	}
	parent, err := parentContext(*configuration.Project)
	if err != nil {
		return v3Plan{}, err
	}
	targetMinor := run.Authority.PromotionMutation.TargetDailyBudgetMinor
	if targetMinor < 1 || targetMinor%100 != 0 || targetMinor != run.Authority.BudgetLimitMinor {
		return v3Plan{}, fmt.Errorf("promotion budget is outside the exact authority")
	}
	required := true
	return v3Plan{
		SchemaVersion: rparunner.PlanSchemaV3, PlanKind: v3PromotionEdit, Browser: "msedge", Mode: "prepare", Status: "ready",
		AccountReference: run.AccountID, ObjectReference: run.Authority.TargetPlatformObjectID, ParentProjectReference: run.Authority.ParentPlatformProjectID,
		ParentConditionManifestID: v3ParentManifestID, ParentContext: parent, BlockedReasons: []string{},
		Steps: []v3Step{
			{ID: "001-identify-page", Kind: "identify_page", PageKind: v3PromotionEdit},
			{ID: "002-promotion.daily_budget", Kind: "field_action", PageKind: v3PromotionEdit, FieldKey: "promotion.daily_budget", Operation: "fill_money", Scope: "单元预算", Target: "spinbutton", Value: strconv.FormatInt(targetMinor/100, 10), ValueState: "provided", Required: &required},
			{ID: "003-readback", Kind: "readback", PageKind: v3PromotionEdit},
			{ID: "004-final-click-boundary", Kind: "final_click_boundary", PageKind: v3PromotionEdit, Target: "保存并关闭", RemoteWrite: true, Blocked: true, BlockReason: "PREPARE_PLAN_REMOTE_WRITE_PROHIBITED"},
		},
		AllowRemoteWrite: false, MaximumFinalClicks: 0,
	}, nil
}

func (c V3Compiler) hydrateProductImageEvidence(ctx context.Context, run browserautomation.BrowserRpaRun, configuration delivery.PlatformConfiguration) (delivery.PlatformConfiguration, error) {
	ocean := configuration.Payload.OceanEngine
	if ocean == nil {
		return configuration, nil
	}
	needsHydration := false
	for _, promotion := range ocean.Promotions {
		for _, reference := range promotion.ProductImageReferences {
			if !validImageSourceIdentity(reference.AuditAttributes["image_src_identity"]) {
				needsHydration = true
			}
		}
	}
	if !needsHydration {
		return configuration, nil
	}
	if c.PlatformObjects == nil {
		return configuration, fmt.Errorf("product image Collector evidence is unavailable")
	}

	oceanCopy := *ocean
	oceanCopy.Promotions = append([]delivery.OceanEnginePromotionDraft(nil), ocean.Promotions...)
	for promotionIndex := range oceanCopy.Promotions {
		promotion := &oceanCopy.Promotions[promotionIndex]
		promotion.ProductImageReferences = append([]delivery.StableReference(nil), promotion.ProductImageReferences...)
		for referenceIndex := range promotion.ProductImageReferences {
			reference := &promotion.ProductImageReferences[referenceIndex]
			if validImageSourceIdentity(reference.AuditAttributes["image_src_identity"]) {
				continue
			}
			accountID := strings.TrimPrefix(reference.Scope, "account:")
			if accountID == reference.Scope || strings.TrimSpace(accountID) == "" {
				return configuration, fmt.Errorf("product image %s has no Collector account scope", reference.ID)
			}
			objects, loadErr := c.PlatformObjects.ListPlatformObjects(ctx, connector.PlatformObjectQuery{
				OrganizationID: string(run.OrganizationID), ProjectID: string(run.ProjectID), AccountID: accountID,
				Kind: connector.PlatformObjectProductImage, Status: "active", Search: reference.ID, Limit: 20,
			})
			if loadErr != nil {
				return configuration, fmt.Errorf("load product image %s from Collector: %w", reference.ID, loadErr)
			}
			connectorID := reference.AuditAttributes["connector_platform_object_id"]
			var matched *connector.PlatformObject
			for objectIndex := range objects {
				candidate := &objects[objectIndex]
				if candidate.PlatformObjectID == reference.ID || (connectorID != "" && candidate.ID == connectorID) {
					if matched != nil {
						return configuration, fmt.Errorf("product image %s matched multiple Collector objects", reference.ID)
					}
					matched = candidate
				}
			}
			if matched == nil {
				return configuration, fmt.Errorf("product image %s was not found in the Collector", reference.ID)
			}
			identity, _ := matched.Metadata["web_uri"].(string)
			if !validImageSourceIdentity(identity) {
				return configuration, fmt.Errorf("product image %s has no stable Collector web_uri", reference.ID)
			}
			audit := make(map[string]string, len(reference.AuditAttributes)+1)
			for key, value := range reference.AuditAttributes {
				audit[key] = value
			}
			audit["image_src_identity"] = strings.TrimSpace(identity)
			reference.AuditAttributes = audit
		}
	}
	configuration.Payload.OceanEngine = &oceanCopy
	return configuration, nil
}

func (c V3Compiler) hydrateOptimizationTargetEvidence(ctx context.Context, run browserautomation.BrowserRpaRun, configuration delivery.PlatformConfiguration) (delivery.PlatformConfiguration, error) {
	ocean := configuration.Payload.OceanEngine
	if ocean == nil || ocean.Project == nil || ocean.Project.OptimizationTargetReference == nil {
		return configuration, nil
	}
	reference := ocean.Project.OptimizationTargetReference
	if !strings.HasPrefix(reference.SemanticKey, "external_action:") {
		return configuration, nil
	}
	if c.PlatformObjects == nil {
		return configuration, fmt.Errorf("optimization target Collector evidence is unavailable")
	}
	accountID := strings.TrimPrefix(reference.Scope, "account:")
	if accountID == reference.Scope || strings.TrimSpace(accountID) == "" {
		return configuration, fmt.Errorf("optimization target %s has no Collector account scope", reference.ID)
	}
	objects, err := c.PlatformObjects.ListPlatformObjects(ctx, connector.PlatformObjectQuery{
		OrganizationID: string(run.OrganizationID), ProjectID: string(run.ProjectID), AccountID: accountID,
		Kind: connector.PlatformObjectOptimizationTarget, Status: "active", Search: reference.ID, Limit: 20,
	})
	if err != nil {
		return configuration, fmt.Errorf("load optimization target %s from Collector: %w", reference.ID, err)
	}
	semantic := ""
	displayName := ""
	for _, candidate := range objects {
		if candidate.PlatformObjectID != reference.ID {
			continue
		}
		inferred := optimizationTargetFromDisplayName(candidate.DisplayName)
		if inferred == "" {
			continue
		}
		if semantic != "" {
			return configuration, fmt.Errorf("optimization target %s matched multiple Collector objects", reference.ID)
		}
		semantic, displayName = inferred, candidate.DisplayName
	}
	if semantic == "" {
		return configuration, fmt.Errorf("optimization target %s has no calibrated Collector label", reference.ID)
	}
	oceanCopy := *ocean
	projectCopy := *ocean.Project
	referenceCopy := *reference
	referenceCopy.SemanticKey = semantic
	referenceCopy.DisplayNameSnapshot = displayName
	projectCopy.OptimizationTargetReference = &referenceCopy
	oceanCopy.Project = &projectCopy
	configuration.Payload.OceanEngine = &oceanCopy
	return configuration, nil
}

func blockedConfigurationPlan(run browserautomation.BrowserRpaRun, planKind, parentProjectReference string, availability []V3ObjectAvailability, issue error) v3Plan {
	required := true
	return v3Plan{
		SchemaVersion: rparunner.PlanSchemaV3, PlanKind: planKind, Browser: "msedge", Mode: "prepare", Status: "blocked",
		AccountReference: run.AccountID, ParentProjectReference: parentProjectReference, ParentConditionManifestID: v3ParentManifestID,
		BlockedReasons: []string{invalidDeliveryConfigurationReason}, ConfigurationIssues: []string{issue.Error()}, ObjectAvailability: availability,
		Steps:            []v3Step{{ID: "001-configuration-validation", Kind: "preflight", PageKind: planKind, Required: &required, Blocked: true, BlockReason: invalidDeliveryConfigurationReason}},
		AllowRemoteWrite: false, MaximumFinalClicks: 0,
	}
}

func decodePlannedForm(form V3PlannedForm) (v3Plan, error) {
	var plan v3Plan
	if err := json.Unmarshal(form.Plan, &plan); err != nil {
		return v3Plan{}, fmt.Errorf("decode generated form plan: %w", err)
	}
	if plan.Status != "ready" || len(plan.BlockedReasons) != 0 {
		return v3Plan{}, fmt.Errorf("generated form plan is blocked: %s", strings.Join(plan.BlockedReasons, ","))
	}
	plan.InternalObjectKind = form.InternalObjectKind
	plan.InternalObjectID = form.InternalObjectID
	return plan, nil
}

func (c V3Compiler) stagedBindings(ctx context.Context, run browserautomation.BrowserRpaRun, configuration delivery.PlatformConfiguration) (V3ObjectBindings, error) {
	source, ok := c.Source.(V3MappingSource)
	if !ok {
		return V3ObjectBindings{PromotionPlatformIDs: map[string]string{}}, nil
	}
	mappings, err := source.ListPlatformEntityMappings(ctx, run.OrganizationID, run.ProjectID, run.AccountID)
	if err != nil {
		return V3ObjectBindings{}, fmt.Errorf("load staged platform mappings: %w", err)
	}
	return V3BindingsFromMappings(configuration, run.AccountID, mappings)
}

func nextStagedCreateForm(forms []V3PlannedForm) (V3PlannedForm, error) {
	for _, form := range forms {
		if form.PlatformObjectID == "" {
			return form, nil
		}
	}
	return V3PlannedForm{}, fmt.Errorf("all configured platform objects already have confirmed mappings")
}

func nextUnboundPromotionForm(forms []V3PlannedForm) (V3PlannedForm, error) {
	for _, form := range forms {
		if form.InternalObjectKind == "promotion" && form.PlatformObjectID == "" {
			return form, nil
		}
	}
	return V3PlannedForm{}, fmt.Errorf("all configured promotions already have confirmed mappings")
}

func parentContext(project delivery.OceanEngineProjectDraft) (v3ParentContext, error) {
	optimization := ""
	externalAction := ""
	if project.OptimizationTargetReference != nil {
		externalAction = strings.TrimSpace(project.OptimizationTargetReference.ID)
		optimization = strings.TrimSpace(project.OptimizationTargetReference.SemanticKey)
		if optimization == "" {
			optimization = optimizationTargetFromDisplayName(project.OptimizationTargetReference.DisplayNameSnapshot)
		}
		if optimization == "" {
			optimization = externalAction
		}
	}
	optimization = normalizedOptimizationTarget(optimization)
	if optimization == "" {
		return v3ParentContext{}, fmt.Errorf("configuration has no calibrated optimization target key")
	}
	if project.MarketingPurpose == "lead_generation" && (project.OptimizationTargetReference == nil || strings.TrimSpace(project.OptimizationTargetReference.AuditAttributes["capability_snapshot_id"]) == "" || strings.TrimSpace(project.OptimizationTargetReference.AuditAttributes["capability_context_hash"]) == "") {
		return v3ParentContext{}, fmt.Errorf("lead-generation optimization target has no account capability snapshot")
	}
	deep := strings.TrimSpace(project.DeepOptimizationMode)
	if deep == "" {
		deep = "disabled"
	}
	deliveryMode := strings.TrimSpace(project.DeliveryMode)
	if deliveryMode == "automatic" {
		deliveryMode = "ubmax"
	}
	// The lead-generation project form does not expose a delivery-mode input.
	// OceanEngine fixes this branch to delivery_mode=3 (UBMax).
	if project.MarketingPurpose == "lead_generation" {
		deliveryMode = "ubmax"
	}
	placementMode := strings.TrimSpace(project.PlacementStrategy)
	if placementMode == "" {
		placementMode = "automatic"
	}
	parentReferences := map[string]string{}
	if project.ApplicationReference != nil && numericReference(project.ApplicationReference.ID) {
		parentReferences["byte_miniapp_reference"] = project.ApplicationReference.ID
	}
	searchExpansion := false
	if project.SearchBoost != nil && project.SearchBoost.TargetingExpansion != nil {
		searchExpansion = *project.SearchBoost.TargetingExpansion
	}
	return v3ParentContext{
		Carrier: project.Carrier, OptimizationTarget: optimization, OptimizationTargetExternalAction: externalAction, DeepOptimization: deep,
		DeliveryMode: deliveryMode, PlacementMode: placementMode,
		SearchTargetingExpansion: searchExpansion, ParentReferences: parentReferences,
	}, nil
}

func optimizationTargetFromDisplayName(value string) string {
	return map[string]string{
		"按钮跳转":   "button_jump",
		"app内下单": "in_app_order",
		"点击量":    "click",
		"展示量":    "impression",
		"门店电话拨打": "store_call",
		"门店停留":   "store_stay",
	}[strings.TrimSpace(value)]
}

func normalizedOptimizationTarget(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "button_redirect", "builtin:button_redirect":
		return "button_jump"
	default:
		return value
	}
}

func numericReference(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func (c V3Compiler) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

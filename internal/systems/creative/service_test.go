package creative

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/media"
)

func TestManualIntakeNeedsClarificationBeforeCreatingTask(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-1", CreateIntakeRequest{
		Source: IntakeSourceManual, Channel: ChannelXiaohongshu, Tone: []string{}, VisualKeywords: []string{}, Mandatory: []string{}, Prohibited: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if intake.Status != IntakeNeedsClarification || len(intake.MissingFields) != 3 {
		t.Fatalf("intake = %#v", intake)
	}
	if _, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest()); !errors.Is(err, ErrIntakeNotReady) {
		t.Fatalf("error = %v, want %v", err, ErrIntakeNotReady)
	}
}

func TestManualReadyIntakeCreatesImageTextTaskAndDraft(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-2", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	if intake.Status != IntakeReady || intake.ConfirmedBy != "usr_1" {
		t.Fatalf("intake = %#v", intake)
	}
	task, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest())
	if err != nil {
		t.Fatal(err)
	}
	if task.Format != FormatImageText || task.Channel != ChannelXiaohongshu {
		t.Fatalf("task = %#v", task)
	}
	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Draft.TitleCandidates) != 3 || len(detail.Draft.ImagePlan) != 4 || detail.Draft.CoverCopy == "" {
		t.Fatalf("draft = %#v", detail.Draft)
	}
}

func TestListVersionsAndPackagesRestoresDeliveredCreativeAfterRefresh(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	now := time.Date(2026, time.July, 24, 10, 0, 0, 0, time.UTC)
	version := CreativeVersion{
		ID: "creativeversion_1", OrganizationID: "org_1", ProjectID: "project_1",
		TaskID: "creativetask_1", Version: 2, DraftVersion: 2,
		Status: CreativeVersionApproved, CreatedAt: now,
	}
	pkg := CreativePackage{
		ID: "creativepackage_1", OrganizationID: "org_1", ProjectID: "project_1",
		CreativeVersionID: version.ID, CreatedAt: now,
	}
	repository.versions[version.ID] = version
	repository.packages[pkg.ID] = pkg

	versions, err := service.ListVersions(context.Background(), testRequestContext().Actor, "project_1", "creativetask_1", 20)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := service.ListPackages(context.Background(), testRequestContext().Actor, "project_1", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != version.ID || len(packages) != 1 || packages[0].ID != pkg.ID {
		t.Fatalf("versions=%#v packages=%#v", versions, packages)
	}
}

func TestApprovedStrategyPackageCreatesReadyCreativeIntake(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{
		PackageID: "package_1", PackageVersion: 2, ContentHash: "sha256:package", CreativeReady: true,
		Objective: "建立新品认知", Audience: "关注生活方式的上班族", CoreMessage: "一杯咖啡也可以成为从容开始的仪式",
		Concept: "温暖晨光中的咖啡桌", Tone: []string{"自然"}, VisualKeywords: []string{"晨光"}, Mandatory: []string{}, Prohibited: []string{},
	}}
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-strategy", CreateIntakeRequest{
		Source: IntakeSourceStrategyPackage, StrategyPackage: &StrategyPackageReference{PackageID: "package_1", PackageVersion: 2, ExpectedContentHash: "sha256:package"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if intake.Source != IntakeSourceStrategyPackage || intake.Status != IntakeReady || intake.Request.Objective == "" {
		t.Fatalf("intake = %#v", intake)
	}
	if _, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest()); err != nil {
		t.Fatalf("strategy intake did not create Creative task: %v", err)
	}
}

func TestCreateStrategyIntakeDeduplicatesTheSamePackageVersion(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{PackageID: "package_1", PackageVersion: 1, ContentHash: "hash", CreativeReady: true, Objective: "目标", Audience: "受众", CoreMessage: "主张", Concept: "概念", Tone: []string{}, VisualKeywords: []string{}, Mandatory: []string{}, Prohibited: []string{}}}
	rc := testRequestContext()
	request := CreateIntakeRequest{Source: IntakeSourceStrategyPackage, StrategyPackage: &StrategyPackageReference{PackageID: "package_1", PackageVersion: 1, ExpectedContentHash: "hash"}}
	first, err := service.CreateIntake(context.Background(), rc, "project_1", "strategy-intake-first", request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIntake(context.Background(), rc, "project_1", "strategy-intake-second", request)
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Fatalf("same strategy package should return its existing Intake: first=%q second=%q", first.ID, second.ID)
	}
}

func TestStrategyPackageWithoutCreativeReadinessNeedsClarification(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{CreativeReady: false, Objective: "目标", Audience: "受众", CoreMessage: "主张", Concept: "概念", Tone: []string{}, VisualKeywords: []string{}, Mandatory: []string{}, Prohibited: []string{}}}
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-not-ready", CreateIntakeRequest{Source: IntakeSourceStrategyPackage, StrategyPackage: &StrategyPackageReference{PackageID: "package_2", PackageVersion: 1, ExpectedContentHash: "hash"}})
	if err != nil {
		t.Fatal(err)
	}
	if intake.Status != IntakeNeedsClarification || len(intake.MissingFields) != 1 || intake.MissingFields[0] != "strategy_package.creative_ready" {
		t.Fatalf("intake = %#v", intake)
	}
}

func TestReadySelectedRouteCanProceedWhenPackageHasOptionalContextBlockers(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{
		PackageID: "package_route_ready", PackageVersion: 1, ContentHash: "sha256:package", CreativeReady: false,
		Objective: "建立品牌认知", Audience: "研发负责人", CoreMessage: "精度可以被验证",
		Tone: []string{}, VisualKeywords: []string{}, Mandatory: []string{}, Prohibited: []string{},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: "route_xhs_ready", RouteType: CreativeRouteImageText,
			Channels: []string{string(ChannelXiaohongshu)}, Reason: "小红书图文路线已确认",
			AspectRatio: "3:4", ReadinessStatus: "ready",
		}},
	}}
	request := CreateIntakeRequest{
		ContractVersion: CreativeIntakeCreateV3ContractVersion,
		Source:          IntakeSourceStrategyPackage,
		StrategyPackage: &StrategyPackageReference{
			PackageID: "package_route_ready", PackageVersion: 1, ExpectedContentHash: "sha256:package",
			HandoffContractVersion: "strategy-creative-handoff/v1", ExpectedHandoffHash: "sha256:handoff",
		},
		SelectedRouteID: "route_xhs_ready",
	}
	intake, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "route-ready", request)
	if err != nil {
		t.Fatal(err)
	}
	if intake.Status != IntakeReady || len(intake.MissingFields) != 0 {
		t.Fatalf("ready selected route should be sufficient for handoff: %#v", intake)
	}
}

func TestReadySelectedRouteReconcilesAnEarlierBlockedIntake(t *testing.T) {
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{
		PackageID: "package_route_reconcile", PackageVersion: 1, ContentHash: "sha256:package", CreativeReady: false,
		Objective: "建立品牌认知", Audience: "研发负责人", CoreMessage: "精度可以被验证",
		Tone: []string{}, VisualKeywords: []string{}, Mandatory: []string{}, Prohibited: []string{},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: "route_brand_ready", RouteType: CreativeRouteBrandVideo, VideoPurpose: "brand",
			Channels: []string{string(ChannelDouyin)}, Reason: "品牌广告路线已确认",
			TargetDurationSeconds: 30, AspectRatio: "16:9", ReadinessStatus: "ready", RequiresHumanConfirmation: true,
		}},
	}}
	request := CreateIntakeRequest{
		ContractVersion: CreativeIntakeCreateV3ContractVersion,
		Source:          IntakeSourceStrategyPackage,
		StrategyPackage: &StrategyPackageReference{
			PackageID: "package_route_reconcile", PackageVersion: 1, ExpectedContentHash: "sha256:package",
			HandoffContractVersion: "strategy-creative-handoff/v1", ExpectedHandoffHash: "sha256:handoff",
		},
		SelectedRouteID: "route_brand_ready",
	}
	first, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "route-reconcile-first", request)
	if err != nil {
		t.Fatal(err)
	}
	repository := service.Repository.(*memoryRepository)
	blocked := first
	blocked.Status = IntakeNeedsClarification
	blocked.MissingFields = []string{"strategy_package.creative_ready"}
	blocked.ConfirmedBy = ""
	repository.intakes[first.ID] = blocked

	reconciled, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "route-reconcile-second", request)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.ID != first.ID || reconciled.Status != IntakeReady || len(reconciled.MissingFields) != 0 || reconciled.Version != first.Version+1 {
		t.Fatalf("existing intake was not safely reconciled: %#v", reconciled)
	}
}

func TestBrandStrategyHandoffNeedsNoOverlayAndDoesNotInventCreativeFields(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{
		PackageID: "package_brand", PackageVersion: 3, ContentHash: "sha256:package", CreativeReady: true,
		Objective: "build brand recognition", Audience: "operations leaders", CoreMessage: "automation returns judgment to people",
		CallToAction: "learn more", Concept: "strategy-authored concept", Tone: []string{"credible"},
		VisualKeywords: []string{"strategy-authored visual"}, Mandatory: []string{"show the product"}, Prohibited: []string{"no fabricated claims"},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: "route_brand_video", RouteType: CreativeRouteBrandVideo, VideoPurpose: "brand",
			Channels: []string{"douyin", "xiaohongshu"}, Reason: "approved brand-video route",
			TargetDurationSeconds: 30, AspectRatio: "9:16", ReadinessStatus: "ready", RequiresHumanConfirmation: true,
		}},
	}}
	request := CreateIntakeRequest{
		ContractVersion: CreativeIntakeCreateV3ContractVersion, Source: IntakeSourceStrategyPackage,
		StrategyPackage: &StrategyPackageReference{
			PackageID: "package_brand", PackageVersion: 3, ExpectedContentHash: "sha256:package",
			HandoffContractVersion: "strategy-creative-handoff/v1", ExpectedHandoffHash: "sha256:handoff",
		},
		SelectedRouteID: "route_brand_video",
	}

	intake, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "brand-no-overlay", request)
	if err != nil {
		t.Fatal(err)
	}
	if intake.Status != IntakeReady || intake.Request.TaskOverlay != nil || intake.Request.TaskOverlayInput != nil {
		t.Fatalf("optional overlay unexpectedly blocked brand handoff: %#v", intake)
	}
	if intake.Request.Format != FormatVideo || intake.Request.PerformanceMode != CreativeRouteBrandVideo || intake.Request.Channel != "" {
		t.Fatalf("brand routing projection = %#v", intake.Request)
	}
	if intake.Request.CallToAction != "" || intake.Request.Concept != "" || len(intake.Request.VisualKeywords) != 0 {
		t.Fatalf("Strategy invented Creative-owned fields: %#v", intake.Request)
	}
	if intake.Request.Objective == "" || intake.Request.Audience == "" || intake.Request.CoreMessage == "" ||
		len(intake.Request.Tone) != 1 || len(intake.Request.Mandatory) != 1 || len(intake.Request.Prohibited) != 1 {
		t.Fatalf("approved brand facts were not projected: %#v", intake.Request)
	}
}

func TestBrandStrategyHandoffRejectsRouteWithoutProductionChannel(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{
		PackageID: "package_brand_unsupported", PackageVersion: 1, ContentHash: "sha256:package", CreativeReady: true,
		Objective: "brand", Audience: "audience", CoreMessage: "message",
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: "route_brand_video", RouteType: CreativeRouteBrandVideo, VideoPurpose: "brand",
			Channels: []string{"wechat_official_account"}, Reason: "approved but not producible",
			TargetDurationSeconds: 30, AspectRatio: "9:16", ReadinessStatus: "ready", RequiresHumanConfirmation: true,
		}},
	}}
	_, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "brand-unsupported", CreateIntakeRequest{
		ContractVersion: CreativeIntakeCreateV3ContractVersion, Source: IntakeSourceStrategyPackage,
		StrategyPackage: &StrategyPackageReference{
			PackageID: "package_brand_unsupported", PackageVersion: 1, ExpectedContentHash: "sha256:package",
			HandoffContractVersion: "strategy-creative-handoff/v1", ExpectedHandoffHash: "sha256:handoff",
		},
		SelectedRouteID: "route_brand_video",
	})
	if err == nil || !strings.Contains(err.Error(), "no Creative-supported production channel") {
		t.Fatalf("unsupported brand route error = %v", err)
	}
}

func TestTaskStrategyHandoffCreatesFrozenReadyIntake(t *testing.T) {
	t.Parallel()
	service := testService()
	service.TaskStrategies = taskStrategyReader{snapshot: TaskStrategySnapshot{
		PlanID: "plan_1", StrategyVersion: 2, ContentHash: "sha256:task",
		BusinessCode: BusinessXiaohongshuImageText,
		Objective:    "建立新品认知", Audience: TaskStrategyAudience{Primary: "通勤女性", Insights: []string{"关注便携"}},
		CoreMessage: "轻盈气泡水", CallToAction: "收藏内容", Concept: "通勤场景种草",
		Tone: []string{"清爽"}, VisualKeywords: []string{"自然光"},
		BusinessStrategy: map[string]any{"content_angle": "通勤补水"},
		MessageHierarchy: []string{"场景", "利益点", "证据"}, ClaimsAndEvidence: []string{"0 糖"},
		Guardrails: []string{"不得夸大"}, Media: []TaskStrategyMediaItem{},
		ReferenceUse:  TaskStrategyReferenceUse{RightsStatus: "unknown", IntendedUse: "style_description", Warnings: []string{}},
		OpenQuestions: []string{"确认商业内容披露"},
		Lineage:       TaskStrategyLineage{BriefID: "brief_1", BriefVersion: 1, BriefContentHash: "sha256:brief"},
	}}
	rc := testRequestContext()
	request := CreateIntakeRequest{
		Source: IntakeSourceTaskStrategy,
		TaskStrategy: &TaskStrategyReference{
			PlanID: "plan_1", StrategyVersion: 2, ExpectedContentHash: "sha256:task",
		},
	}
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "task-strategy-handoff", request)
	if err != nil {
		t.Fatal(err)
	}
	if intake.Source != IntakeSourceTaskStrategy || intake.Status != IntakeReady ||
		intake.Request.TaskStrategyInput == nil ||
		intake.Request.TaskStrategyInput.BusinessCode != BusinessXiaohongshuImageText ||
		intake.Request.Concept != "通勤场景种草" {
		t.Fatalf("intake = %#v", intake)
	}
	task, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest())
	if err != nil {
		t.Fatal(err)
	}
	if task.IntakeID != intake.ID || task.Format != FormatImageText {
		t.Fatalf("task = %#v", task)
	}
}

func TestTaskStrategyHandoffCreatesReadyBrandIntake(t *testing.T) {
	t.Parallel()
	service := testService()
	service.TaskStrategies = taskStrategyReader{snapshot: TaskStrategySnapshot{
		PlanID: "plan_brand", StrategyVersion: 1, ContentHash: "sha256:brand",
		BusinessCode: BusinessBrandVideo, Objective: "品牌认知",
		Audience: TaskStrategyAudience{Primary: "大众"}, CoreMessage: "品牌主张",
		BusinessStrategy: map[string]any{}, Media: []TaskStrategyMediaItem{},
	}}
	intake, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "brand-handoff", CreateIntakeRequest{
		Source: IntakeSourceTaskStrategy,
		TaskStrategy: &TaskStrategyReference{
			PlanID: "plan_brand", StrategyVersion: 1, ExpectedContentHash: "sha256:brand",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if intake.ContractVersion != CreativeIntakeV3ContractVersion || intake.Status != IntakeReady ||
		intake.InputIdentityHash == "" || intake.Request.SelectedRouteID == "" || len(intake.Request.CreativeRoutes) != 1 ||
		intake.Request.CreativeRoutes[0].RouteType != CreativeRouteBrandVideo ||
		intake.Request.CreativeRoutes[0].VideoPurpose != "brand" || intake.Request.Channel != ChannelXiaohongshu {
		t.Fatalf("brand task-strategy intake = %#v", intake)
	}
}

func TestTaskStrategyCallerCannotSubmitMappedContent(t *testing.T) {
	t.Parallel()
	request := CreateIntakeRequest{
		Source: IntakeSourceTaskStrategy,
		TaskStrategy: &TaskStrategyReference{
			PlanID: "plan_1", StrategyVersion: 1, ExpectedContentHash: "hash",
		},
		Objective: "forged",
	}
	if err := request.Validate(); err == nil {
		t.Fatal("expected caller-supplied mapped fields to be rejected")
	}
}

func TestCreativeBusinessCapabilitiesExposeOnlyImplementedHandoffs(t *testing.T) {
	t.Parallel()
	values := CreativeBusinessCapabilities()
	available := map[string]bool{}
	for _, value := range values {
		if value.Status == "available" {
			available[value.BusinessCode] = true
		}
	}
	for _, code := range []string{
		BusinessXiaohongshuImageText, BusinessShortDramaPreroll,
		BusinessCommercePreroll, BusinessViralRemake, BusinessBrandVideo,
	} {
		if !available[code] {
			t.Fatalf("%s should be available: %#v", code, values)
		}
	}
	if available[BusinessGamePreroll] || available[BusinessWechatArticle] {
		t.Fatalf("preview-only businesses must not be available: %#v", available)
	}
}

func TestManualProductionIntakeKeepsCompatibleTaskStrategyParent(t *testing.T) {
	t.Parallel()
	newRequest := func(parentID string) CreateIntakeRequest {
		return CreateIntakeRequest{
			Source: IntakeSourceManual, ParentIntakeID: parentID,
			Format: FormatVideo, PerformanceMode: PerformanceModeViralRemake,
			Channel: ChannelDouyin, Objective: "create an original conversion ad",
			Audience: "efficiency tool users", CoreMessage: "reduce repetitive work",
			CallToAction: "try now", Concept: "reuse the mechanism, not the expression",
			Tone: []string{"clear"}, VisualKeywords: []string{"high contrast"},
			Mandatory: []string{}, Prohibited: []string{},
			CreativeRoutes: []CreativeRouteSnapshot{{
				RouteID: ManualViralRemakeRouteID, RouteType: PerformanceModeViralRemake,
				VideoPurpose: "performance", Channels: []string{"douyin"},
				Reason:                "user selected the task strategy handoff",
				TargetDurationSeconds: 15, AspectRatio: "9:16", RequiresHumanConfirmation: true,
			}},
			ManualViralRemake: &ManualViralRemakeInput{
				ProductName: "FlowKit", SellingPoints: []string{"reduce repetitive work"},
				UserInstruction:      "keep only the transferable pacing mechanism",
				ReferenceVideo:       contract.AssetVersionRef{AssetID: "asset_reference_video", Version: 1},
				ReferenceVideoRights: RightsPending,
			},
		}
	}

	service := testService()
	repository := service.Repository.(*memoryRepository)
	repository.intakes["parent_viral"] = CreativeIntake{
		ID: "parent_viral", OrganizationID: "org_1", ProjectID: "project_1",
		Source: IntakeSourceTaskStrategy, Status: IntakeReady,
		Request: CreateIntakeRequest{TaskStrategyInput: &TaskStrategyInput{
			ContractVersion: TaskStrategyContractVersion, BusinessCode: BusinessViralRemake,
		}},
	}
	intake, err := service.CreateIntake(
		context.Background(), testRequestContext(), "project_1", "viral-production-child",
		newRequest("parent_viral"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if intake.Request.ParentIntakeID != "parent_viral" {
		t.Fatalf("parent lineage was lost: %#v", intake.Request)
	}

	repository.intakes["parent_commerce"] = CreativeIntake{
		ID: "parent_commerce", OrganizationID: "org_1", ProjectID: "project_1",
		Source: IntakeSourceTaskStrategy, Status: IntakeReady,
		Request: CreateIntakeRequest{TaskStrategyInput: &TaskStrategyInput{
			ContractVersion: TaskStrategyContractVersion, BusinessCode: BusinessCommercePreroll,
		}},
	}
	_, err = service.CreateIntake(
		context.Background(), testRequestContext(), "project_1", "invalid-production-child",
		newRequest("parent_commerce"),
	)
	if err == nil || !strings.Contains(err.Error(), "not a compatible") {
		t.Fatalf("expected incompatible parent to be rejected, got %v", err)
	}
}

func TestIntakeIdempotencyDoesNotCreateAnotherIntake(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	first, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-3", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-3", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("idempotency IDs = %q and %q", first.ID, second.ID)
	}
}

func TestTaskCreationAllowsSeveralDistinctDirectionsForTheSameIntake(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-4", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest())
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, CreateTaskRequest{ContentType: ContentTypeIngredientExplanation, Focus: "成分解释"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || first.Direction.ContentType == second.Direction.ContentType {
		t.Fatalf("task directions were not created separately: first=%#v second=%#v", first, second)
	}
}

func TestArchiveTaskHidesItFromActiveQueueButRetainsItsLineage(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-archive", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterCoverImageJob(context.Background(), rc.Actor, "project_1", task.ID, "provider_job_1"); err != nil {
		t.Fatal(err)
	}
	if err := service.ArchiveTask(context.Background(), rc.Actor, "project_1", task.ID); err != nil {
		t.Fatal(err)
	}
	active, err := service.ListTasks(context.Background(), rc.Actor, "project_1", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 {
		t.Fatalf("active tasks = %#v, want archived task omitted", active)
	}
	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Task.Status != TaskArchived || len(detail.ProductionJobs) != 1 || detail.Draft.TaskID != task.ID {
		t.Fatalf("archived detail should retain lineage: %#v", detail)
	}
}

func TestRenameTaskPreservesDraftHistoryAndRequiresCurrentVersion(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-intake-rename", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest())
	if err != nil {
		t.Fatal(err)
	}
	renamed, err := service.RenameTask(context.Background(), rc.Actor, "project_1", task.ID, RenameTaskRequest{ExpectedVersion: task.Version, DisplayName: "娇兰黄金复原蜜 15 秒广告"})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.DisplayName != "娇兰黄金复原蜜 15 秒广告" || renamed.Version != task.Version+1 {
		t.Fatalf("renamed task = %#v", renamed)
	}
	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Draft.Version != 1 {
		t.Fatalf("rename changed draft history: %#v", detail.Draft)
	}
	if _, err := service.RenameTask(context.Background(), rc.Actor, "project_1", task.ID, RenameTaskRequest{ExpectedVersion: task.Version, DisplayName: "旧浏览器覆盖"}); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale rename error = %v, want %v", err, ErrVersionConflict)
	}
}

func TestImagePlanRetriesRetainEachProviderAttempt(t *testing.T) {
	t.Parallel()
	service := testService()
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "creative-image-retry-intake", validManualRequest())
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateTask(context.Background(), rc.Actor, "project_1", intake.ID, defaultTaskRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterImagePlanJob(context.Background(), rc.Actor, "project_1", task.ID, 2, "provider_job_first"); err != nil {
		t.Fatal(err)
	}
	if err := service.RegisterImagePlanJob(context.Background(), rc.Actor, "project_1", task.ID, 2, "provider_job_retry"); err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ProductionJobs) != 2 || detail.ProductionJobs[0].Kind == detail.ProductionJobs[1].Kind {
		t.Fatalf("production retries = %#v", detail.ProductionJobs)
	}
}

func TestCreateVideoTaskConsumesApprovedRouteAndReadyProjectVideo(t *testing.T) {
	t.Parallel()
	service := testService()
	service.StrategyPackages = strategyPackageReader{snapshot: StrategyPackageSnapshot{
		PackageID: "package_1", PackageVersion: 1, ContentHash: "hash_1", CreativeReady: true,
		Objective: "转化", Audience: "短剧观众", CoreMessage: "五秒建立产品利益点", Concept: "先看利益点再进正片",
		Tone: []string{"直接"}, VisualKeywords: []string{"竖屏"}, Mandatory: []string{}, Prohibited: []string{},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteType: "pre_roll", VideoPurpose: "performance", Channels: []string{"douyin"},
			Reason: "正片前快速建立关联", TargetDurationSeconds: 5, AspectRatio: "9:16",
			SourceAssetRefs: []contract.AssetVersionRef{}, EvidenceRefs: []string{}, RequiresHumanConfirmation: true,
		}},
	}}
	service.Assets = testAssetReader{snapshot: CreativeAssetSnapshot{
		Ref: contract.AssetVersionRef{AssetID: "asset_main", Version: 1}, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
	}}
	rc := testRequestContext()
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "video-intake-1", CreateIntakeRequest{
		Source:          IntakeSourceStrategyPackage,
		StrategyPackage: &StrategyPackageReference{PackageID: "package_1", PackageVersion: 1, ExpectedContentHash: "hash_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	task, err := service.CreateVideoTask(context.Background(), rc.Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		RouteIndex: 0, Channel: ChannelDouyin,
		SourceVideo: contract.AssetVersionRef{AssetID: "asset_main", Version: 1},
		Concept:     "利益点前贴", Prompt: "竖屏五秒产品利益点广告", CallToAction: "继续观看",
		Mandatory: []string{}, Prohibited: []string{}, ConfirmRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Format != FormatVideo || task.PerformanceMode != "pre_roll" || detail.VideoDraft == nil ||
		detail.VideoDraft.SourceVideo.AssetID != "asset_main" || detail.VideoDraft.DurationSeconds != 5 {
		t.Fatalf("unexpected video task detail: %+v", detail)
	}
}

func TestCreateBrandVideoTaskConsumesConfirmedDirectionWithoutReferenceVideo(t *testing.T) {
	t.Parallel()
	service := testService()
	intake := CreativeIntake{
		ContractVersion: CreativeIntakeV3ContractVersion,
		ID:              "intake_brand", OrganizationID: "org_1", ProjectID: "project_1",
		Source: IntakeSourceStrategyPackage, Status: IntakeReady,
		InputIdentityHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Request: CreateIntakeRequest{
			Source: IntakeSourceStrategyPackage, SelectedRouteID: "route_brand_video",
			Audience: "研发与采购负责人", CoreMessage: "把不可见的风险变成可验证的工程判断",
			CreativeRoutes: []CreativeRouteSnapshot{{
				RouteID: "route_brand_video", RouteType: CreativeRouteBrandVideo, VideoPurpose: "brand",
				Channels: []string{"xiaohongshu"}, Reason: "建立品牌认知", TargetDurationSeconds: 30,
				AspectRatio: "9:16", Resolution: "1080x1920", RequiresHumanConfirmation: true, ReadinessStatus: "ready",
			}},
		},
	}
	service.Repository.(*memoryRepository).intakes[intake.ID] = intake
	service.BrandBriefs = confirmedBrandBriefRepository(intake)
	brandBriefRepository := service.BrandBriefs.(*brandBriefRepositoryStub)
	brandBriefRepository.review.ContentHash = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	brandBriefRepository.review.Document = BrandBriefDocument{
		Summary:          "面向研发与采购负责人的品牌认知广告",
		AudienceSegments: []BrandBriefAudience{{SegmentID: "audience_1", Label: "研发与采购负责人", Insight: "需要可验证证据"}},
		Product:          BrandBriefProduct{BrandName: "灵裁", ProductName: "智能创作平台", SellingPoints: []string{"把风险变成工程判断"}, ProofPoints: []string{"evidence_1"}},
		Communication:    BrandBriefCommunication{SingleMindedProposition: intake.Request.CoreMessage, ToneConstraints: []string{"克制可信"}},
		Guardrails:       []BrandBriefGuardrail{{GuardrailID: "guardrail_1", Text: "不虚构结论"}},
		Route:            BrandBriefRoute{RouteID: "route_brand_video", Spec: BrandBriefRouteSpec{TargetDurationSeconds: 30, AspectRatio: "9:16", Resolution: "1080x1920"}},
		AudioIntent:      BrandBriefAudioIntent{VoiceDirection: "冷静、可信"},
	}
	brandBrief := brandBriefRepository.review
	createdAt := time.Now().UTC()
	brandBriefRepository.review.CreatedAt = createdAt
	brandBriefRepository.review.UpdatedAt = createdAt
	direction := CreativeDirectionVersion{
		ContractVersion: CreativeDirectionVersionV1, ID: "direction_brand", OrganizationID: "org_1", ProjectID: "project_1",
		BatchID: "batch_brand", IntakeID: intake.ID, InputIdentityHash: intake.InputIdentityHash, RouteID: "route_brand_video",
		Concept: "毫米之间，有人回答", CreativeRationale: "用人物接力建立工程伙伴认知",
		MessagePlan: []string{"问题被接住"}, ExecutionOutline: []string{"动作匹配剪辑"}, GuardrailTrace: []string{"不虚构结论"},
		DirectionMode: "cinematic", EmotionalArc: "从悬而未决到获得回应", VisualGrammar: "工业微距",
		BrandMemoryDevice: "银色光带", HumanMoment: "隔屏共同确认", Status: DirectionStatusConfirmed,
		Version: 1, ContentHash: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", CreatedAt: createdAt,
		BrandBriefRef: &BrandBriefReference{Revision: brandBrief.Revision, ContentHash: brandBrief.ContentHash},
	}
	alternate := direction
	alternate.ID = "direction_brand_alternate"
	alternate.Concept = "看得见的工程判断"
	alternate.ContentHash = "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	alternate.Status = DirectionStatusCandidate
	service.Directions = &directionRepositoryStub{batch: CreativeDirectionBatch{
		ContractVersion: CreativeDirectionBatchV1, ID: "batch_brand", OrganizationID: "org_1", ProjectID: "project_1",
		IntakeID: intake.ID, InputIdentityHash: intake.InputIdentityHash,
		BrandBriefRef: &BrandBriefReference{Revision: brandBrief.Revision, ContentHash: brandBrief.ContentHash},
		Status:        DirectionBatchReady, Candidates: []CreativeDirectionVersion{direction, alternate},
		Model: "test-model", PromptVersion: "creative-direction/brand-video-v1", CreatedAt: createdAt,
	}}
	task, err := service.CreateVideoTask(context.Background(), testRequestContext().Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: "route_brand_video", DirectionID: direction.ID, Channel: ChannelXiaohongshu,
		Mandatory: []string{}, Prohibited: []string{}, ConfirmRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetTaskDetail(context.Background(), testRequestContext().Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Channel != ChannelXiaohongshu || task.Direction.DirectionVersionID != direction.ID || task.Direction.InputIdentityHash != intake.InputIdentityHash ||
		task.Direction.CallToAction != "" || detail.VideoDraft == nil || detail.VideoDraft.Resolution != "1080x1920" ||
		detail.VideoDraft.BrandFilm == nil || detail.VideoDraft.BrandFilm.Stage != BrandFilmConceptConfirmed ||
		detail.VideoDraft.BrandFilm.SelectedConceptID != direction.ID || !strings.Contains(detail.VideoDraft.Prompt, direction.BrandMemoryDevice) {
		t.Fatalf("brand task did not preserve confirmed direction lineage and route spec: task=%+v detail=%+v", task, detail)
	}
	replayed, err := service.CreateVideoTask(context.Background(), testRequestContext().Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: "route_brand_video", DirectionID: direction.ID, Channel: ChannelXiaohongshu,
		Mandatory: []string{}, Prohibited: []string{}, ConfirmRoute: true,
	})
	if err != nil || replayed.ID != task.ID {
		t.Fatalf("brand task lineage was not idempotent: first=%s replay=%s err=%v", task.ID, replayed.ID, err)
	}
	repository := service.Repository.(*memoryRepository)
	service.ViralRemakes = repository
	legacy := repository.tasks[task.ID]
	legacy.VideoDraft.BrandFilm = nil
	repository.tasks[task.ID] = legacy
	recovered, err := service.InitializeStrategyBrandFilmWorkspace(context.Background(), testRequestContext().Actor, "project_1", task.ID)
	if err != nil || recovered.VideoDraft == nil || recovered.VideoDraft.BrandFilm == nil ||
		recovered.VideoDraft.BrandFilm.SelectedConceptID != direction.ID || recovered.VideoDraft.Revision != 2 {
		t.Fatalf("legacy strategy brand task was not upgraded in place: detail=%+v err=%v", recovered, err)
	}
	recoveredAgain, err := service.InitializeStrategyBrandFilmWorkspace(context.Background(), testRequestContext().Actor, "project_1", task.ID)
	if err != nil || recoveredAgain.VideoDraft.Revision != recovered.VideoDraft.Revision {
		t.Fatalf("legacy workspace repair was not idempotent: first=%d replay=%d err=%v", recovered.VideoDraft.Revision, recoveredAgain.VideoDraft.Revision, err)
	}
}

func TestCreateBrandVideoTaskMaterializesBrandFilmWithoutPreTaskDirection(t *testing.T) {
	t.Parallel()
	service := testService()
	intake := CreativeIntake{
		ContractVersion: CreativeIntakeV3ContractVersion,
		ID:              "intake_brand", OrganizationID: "org_1", ProjectID: "project_1",
		Source: IntakeSourceStrategyPackage, Status: IntakeReady,
		InputIdentityHash: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Request: CreateIntakeRequest{
			Source: IntakeSourceStrategyPackage, SelectedRouteID: "route_brand_video",
			Audience: "研发与采购负责人", CoreMessage: "把不可见的风险变成可验证的工程判断",
			CreativeRoutes: []CreativeRouteSnapshot{{
				RouteID: "route_brand_video", RouteType: CreativeRouteBrandVideo, VideoPurpose: "brand",
				Channels: []string{"xiaohongshu"}, Reason: "建立品牌认知", TargetDurationSeconds: 30,
				AspectRatio: "9:16", Resolution: "1080x1920", RequiresHumanConfirmation: true, ReadinessStatus: "ready",
			}},
		},
	}
	service.Repository.(*memoryRepository).intakes[intake.ID] = intake
	task, err := service.CreateVideoTask(context.Background(), testRequestContext().Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: "route_brand_video", Channel: ChannelXiaohongshu,
		Mandatory: []string{}, Prohibited: []string{}, ConfirmRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetTaskDetail(context.Background(), testRequestContext().Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Channel != ChannelXiaohongshu || task.Direction.DirectionVersionID != "" || task.Direction.InputIdentityHash != intake.InputIdentityHash ||
		task.Direction.CallToAction != "" || detail.VideoDraft == nil || detail.VideoDraft.Resolution != "1080x1920" ||
		detail.VideoDraft.BrandFilm == nil || detail.VideoDraft.BrandFilm.Stage != BrandFilmWaitingBrief ||
		detail.VideoDraft.BrandFilm.SourceSnapshot.SourceKind != string(IntakeSourceStrategyPackage) ||
		detail.VideoDraft.BrandFilm.SourceSnapshot.IntakeID != intake.ID {
		t.Fatalf("brand task did not materialize the shared BrandFilm workspace: task=%+v detail=%+v", task, detail)
	}
	replayed, err := service.CreateVideoTask(context.Background(), testRequestContext().Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: "route_brand_video", Channel: ChannelXiaohongshu,
		Mandatory: []string{}, Prohibited: []string{}, ConfirmRoute: true,
	})
	if err != nil || replayed.ID != task.ID {
		t.Fatalf("brand task replay should return the existing task: task=%+v err=%v", replayed, err)
	}
}

func TestCreateManualViralRemakeTaskUsesStableRouteAndRestorableSnapshot(t *testing.T) {
	t.Parallel()
	service := testService()
	service.Assets = testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
		"asset_reference_video": {
			Ref:  contract.AssetVersionRef{AssetID: "asset_reference_video", Version: 2},
			Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
		},
		"asset_product_image": {
			Ref:  contract.AssetVersionRef{AssetID: "asset_product_image", Version: 1},
			Kind: contract.AssetImage, MIMEType: "image/png", Ready: true,
		},
	}}
	rc := testRequestContext()
	referenceImage := contract.AssetVersionRef{AssetID: "asset_product_image", Version: 1}
	intake, err := service.CreateIntake(context.Background(), rc, "project_1", "manual-viral-intake-1", CreateIntakeRequest{
		Source: IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeViralRemake,
		Channel: ChannelDouyin, Objective: "复用高停留结构，生成原创转化广告", Audience: "效率工具用户",
		CoreMessage: "减少重复操作", CallToAction: "立即体验", Concept: "保留功能节奏并替换受保护表达",
		Tone: []string{"清晰"}, VisualKeywords: []string{"高反差开场"}, Mandatory: []string{}, Prohibited: []string{},
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: ManualViralRemakeRouteID, RouteType: PerformanceModeViralRemake,
			VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "用户选择爆款复刻",
			TargetDurationSeconds: 15, AspectRatio: "9:16", RequiresHumanConfirmation: true,
		}},
		ManualViralRemake: &ManualViralRemakeInput{
			ProductName: "FlowKit", SellingPoints: []string{"自动整理任务", "减少重复操作"},
			UserInstruction: "保留钩子功能和节奏，替换人物、品牌、字幕和音乐",
			ReferenceVideo:  contract.AssetVersionRef{AssetID: "asset_reference_video", Version: 2},
			ReferenceImage:  &referenceImage, ReferenceVideoRights: RightsConfirmed,
			ReferenceImageRights: RightsConfirmed,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	task, err := service.CreateVideoTask(context.Background(), rc.Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: ManualViralRemakeRouteID, Channel: ChannelDouyin,
		SourceVideo: contract.AssetVersionRef{AssetID: "asset_reference_video", Version: 2},
		Concept:     "原创效率工具广告", Prompt: "等待 Phase 2 真实拆解后生成", CallToAction: "立即体验",
		Mandatory: []string{}, Prohibited: []string{}, ConfirmRoute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetTaskDetail(context.Background(), rc.Actor, "project_1", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.PerformanceMode != PerformanceModeViralRemake || detail.VideoDraft == nil ||
		detail.VideoDraft.ViralRemake == nil {
		t.Fatalf("viral workspace was not persisted: %+v", detail)
	}
	viral := detail.VideoDraft.ViralRemake
	if viral.SelectedRouteID != ManualViralRemakeRouteID || viral.Revision != 1 ||
		viral.InputSnapshot.ReferenceVideo != (contract.AssetVersionRef{AssetID: "asset_reference_video", Version: 2}) {
		t.Fatalf("viral input snapshot = %+v", viral)
	}
	if !viral.Readiness.PlanningReady || viral.Readiness.GenerationReady || viral.Readiness.ProductionReady {
		t.Fatalf("viral readiness = %+v", viral.Readiness)
	}
}

func TestRenderJobPersistsFinalAssetLineage(t *testing.T) {
	t.Parallel()
	service := testService()
	repository := service.Repository.(*memoryRepository)
	now := service.now()
	intake := CreativeIntake{
		ID: "intake_video", OrganizationID: "org_1", ProjectID: "project_1", Status: IntakeReady,
		Request: CreateIntakeRequest{
			StrategyPackage: &StrategyPackageReference{PackageID: "strategy_package_1", PackageVersion: 1, ExpectedContentHash: "sha256:strategy"},
			CreativeRoutes: []CreativeRouteSnapshot{{
				RouteType: "pre_roll", VideoPurpose: "performance", Channels: []string{"douyin"}, Reason: "approved",
				TargetDurationSeconds: 5, AspectRatio: "9:16", RequiresHumanConfirmation: true,
			}},
		},
	}
	repository.intakes[intake.ID] = intake
	task := CreativeTask{
		ID: "task_video", OrganizationID: "org_1", ProjectID: "project_1", IntakeID: intake.ID,
		Format: FormatVideo, Channel: ChannelDouyin, VideoPurpose: "performance", PerformanceMode: "pre_roll",
		Status: TaskDraft, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	draft := VideoDraft{
		ContractVersion: "creative-video-draft/v1", TaskID: task.ID, Revision: 1, Concept: "concept", Prompt: "prompt",
		DurationSeconds: 5, AspectRatio: "9:16", Resolution: "720p",
		SourceVideo: contract.AssetVersionRef{AssetID: "asset_main", Version: 1},
		Mandatory:   []string{}, Prohibited: []string{}, CreatedAt: now,
	}
	if _, err := repository.CreateVideoTask(context.Background(), task, draft); err != nil {
		t.Fatal(err)
	}
	taskDetail := repository.tasks[task.ID]
	taskDetail.ProductionJobs = []ProductionJob{{TaskID: task.ID, Kind: "video_generate", ProviderJobID: "providerjob_video_1", CreatedAt: now}}
	repository.tasks[task.ID] = taskDetail
	service.Assets = testAssetReader{snapshot: CreativeAssetSnapshot{
		Ref: contract.AssetVersionRef{AssetID: "asset_preroll", Version: 1}, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true,
	}}
	scheduler := &testRenderScheduler{}
	writer := &testRenderedAssetWriter{ref: contract.ProjectAssetRef{
		ProjectID: "project_1", AssetVersion: contract.AssetVersionRef{AssetID: "asset_final", Version: 1},
	}}
	service.RenderScheduler = scheduler
	service.Composer = testVideoComposer{}
	service.RenderedAssets = writer
	rc := testRequestContext()
	render, _, err := service.CreateRenderJob(context.Background(), rc, "project_1", task.ID, CreateRenderJobRequest{
		PreRollVideo: contract.AssetVersionRef{AssetID: "asset_preroll", Version: 1},
	}, "render-once")
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.render.ID != render.ID {
		t.Fatalf("render was not durably scheduled: %+v", scheduler.render)
	}
	if err := service.ExecuteRenderJob(context.Background(), "org_1", "project_1", render.ID); err != nil {
		t.Fatal(err)
	}
	stored, err := repository.GetRenderJob(context.Background(), "org_1", "project_1", render.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != RenderSucceeded || stored.OutputAsset == nil || stored.OutputAsset.AssetVersion.AssetID != "asset_final" || writer.renderJobID != render.ID {
		t.Fatalf("render lineage is incomplete: render=%+v writer=%+v", stored, writer)
	}
	version, _, err := service.FreezeVersion(context.Background(), rc, "project_1", task.ID, FreezeVersionRequest{
		DraftVersion: 1, RenderJobID: render.ID,
	}, "freeze-video-once")
	if err != nil {
		t.Fatal(err)
	}
	if version.VideoSnapshot == nil || version.VideoSnapshot.FinalVideo.AssetID != "asset_final" ||
		version.VideoSnapshot.ProviderJobID != "providerjob_video_1" {
		t.Fatalf("video version lineage is incomplete: %+v", version)
	}
	checked, err := service.CheckVersion(context.Background(), rc.Actor, "project_1", version.ID)
	if err != nil || checked.Check == nil || !checked.Check.Passed {
		t.Fatalf("video check failed: version=%+v err=%v", checked, err)
	}
	if _, err := service.ApproveVersion(context.Background(), rc.Actor, "project_1", version.ID); err != nil {
		t.Fatal(err)
	}
	pkg, err := service.DeliverVersion(context.Background(), rc.Actor, "project_1", version.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Format != FormatVideo || pkg.VideoSnapshot == nil || pkg.VideoSnapshot.RenderJobID != render.ID {
		t.Fatalf("delivered package lost video lineage: %+v", pkg)
	}
}

func validManualRequest() CreateIntakeRequest {
	return CreateIntakeRequest{
		Source: IntakeSourceManual, Channel: ChannelXiaohongshu, Objective: "建立新品认知", Audience: "关注生活方式的年轻上班族", CoreMessage: "一杯咖啡，也可以成为从容开始的仪式", CallToAction: "收藏这份晨间灵感",
		Concept: "柔和自然光下的蓝白咖啡桌", Tone: []string{"自然", "克制"}, VisualKeywords: []string{"蓝白", "晨光"}, Mandatory: []string{"产品主体"}, Prohibited: []string{},
	}
}

func TestRequirementSnapshotV4CreatesReadyViralRemakeWithoutStrategyPackage(t *testing.T) {
	t.Parallel()
	service := testService()
	videoRef := contract.AssetVersionRef{AssetID: "asset_reference_video", Version: 2}
	service.Requirements = requirementSnapshotReader{snapshot: RequirementSnapshot{
		Objective: "将高停留结构原创映射到新品转化", DeliverableIntent: PerformanceModeViralRemake,
		ProductOrSubject: "FlowKit", Audience: "效率工具用户", CoreMessage: "减少重复操作",
		SellingPoints: []string{"自动整理任务", "减少重复操作"},
		Constraints:   []string{"不得照搬人物、台词和音乐"}, AssetRefs: []contract.AssetVersionRef{videoRef},
	}}
	service.Assets = testAssetReader{snapshots: map[contract.AssetID]CreativeAssetSnapshot{
		videoRef.AssetID: {Ref: videoRef, Kind: contract.AssetVideo, MIMEType: "video/mp4", Ready: true},
	}}
	hash := "sha256:" + strings.Repeat("a", 64)
	capabilityHash := "sha256:" + strings.Repeat("b", 64)
	request := CreateIntakeRequest{
		ContractVersion:        CreativeIntakeCreateV4ContractVersion,
		Source:                 IntakeSourceRequirement,
		RequirementSnapshotRef: &RequirementSnapshotReference{BriefID: "brief_1", BriefVersion: 3, ContentHash: hash},
		BusinessCapabilityRef:  &BusinessCapabilityReference{BusinessCode: BusinessViralRemake, Version: "1.1.0", ContentHash: capabilityHash},
		SelectedRouteID:        ManualViralRemakeRouteID,
	}
	intake, err := service.CreateIntake(context.Background(), testRequestContext(), "project_1", "requirement-v4-1", request)
	if err != nil {
		t.Fatal(err)
	}
	if intake.ContractVersion != CreativeIntakeV4ContractVersion || intake.Source != IntakeSourceRequirement || intake.Status != IntakeReady ||
		intake.Request.ManualViralRemake == nil || intake.Request.ManualViralRemake.ReferenceVideo != videoRef || intake.InputIdentityHash == "" {
		t.Fatalf("v4 intake was not resolved into a ready viral snapshot: %#v", intake)
	}
	if len(intake.Warnings) != 1 || !strings.Contains(intake.Warnings[0], "权利状态") {
		t.Fatalf("rights warning is missing: %#v", intake.Warnings)
	}

	task, err := service.CreateVideoTask(context.Background(), testRequestContext().Actor, "project_1", intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: ManualViralRemakeRouteID, Channel: ChannelDouyin, SourceVideo: videoRef,
		Concept: "原创效率工具广告", Prompt: "等待参考视频分析", CallToAction: "立即体验", ConfirmRoute: true,
		Mandatory: []string{}, Prohibited: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := service.GetTaskDetail(context.Background(), testRequestContext().Actor, "project_1", task.ID)
	if err != nil || detail.VideoDraft == nil || detail.VideoDraft.ViralRemake == nil || detail.VideoDraft.ViralRemake.InputSnapshot.Source != IntakeSourceRequirement {
		t.Fatalf("v4 intake did not reach the viral workspace: detail=%#v err=%v", detail, err)
	}
}

func TestRequirementSnapshotV4CannotBypassFullBrandStrategy(t *testing.T) {
	t.Parallel()
	hash := "sha256:" + strings.Repeat("a", 64)
	request := CreateIntakeRequest{
		ContractVersion:        CreativeIntakeCreateV4ContractVersion,
		Source:                 IntakeSourceRequirement,
		RequirementSnapshotRef: &RequirementSnapshotReference{BriefID: "brief_1", BriefVersion: 1, ContentHash: hash},
		BusinessCapabilityRef:  &BusinessCapabilityReference{BusinessCode: "brand_video", Version: "1.0.0", ContentHash: hash},
		SelectedRouteID:        ManualViralRemakeRouteID,
	}
	if err := request.Validate(); !errors.Is(err, ErrFullStrategyRequired) {
		t.Fatalf("brand path bypass error=%v", err)
	}
}

func TestNonRequirementIntakeRejectsRequirementRefs(t *testing.T) {
	t.Parallel()
	request := validManualRequest()
	request.RequirementSnapshotRef = &RequirementSnapshotReference{
		BriefID: "brief_1", BriefVersion: 1, ContentHash: "sha256:" + strings.Repeat("a", 64),
	}
	if err := request.Validate(); err == nil {
		t.Fatal("manual intake accepted a requirement snapshot ref")
	}
}

func defaultTaskRequest() CreateTaskRequest {
	return CreateTaskRequest{ContentType: ContentTypeLifestyle, Focus: "生活方式种草"}
}

func testRequestContext() contract.RequestContext {
	return contract.RequestContext{RequestID: "req_1", TraceID: "trace_1", Actor: contract.ActorContext{OrganizationID: "org_1", Principal: contract.Principal{Kind: contract.PrincipalUser, ID: "usr_1"}, Scopes: []contract.Scope{ScopeRead, ScopeWrite}}}
}

func TestMandatoryElementSatisfiedUsesSemanticSignalsForStrategyRequirements(t *testing.T) {
	t.Parallel()
	copyText := strings.ToLower("新产品准备打样时，先确认判断依据与资质文件是否可核验；如需讨论具体需求，可私信沟通。")

	for _, requirement := range []string{
		"明确触发场景",
		"可核验或待核验的证据位",
		"低摩擦咨询动作",
		"必须包含明确触发场景、可核验/待核验证据位、低摩擦咨询动作三个强制元素",
		"不得虚构客户案例、设备数量或交付承诺",
		"仅使用已绑定研究证据描述行业决策逻辑",
	} {
		if !mandatoryElementSatisfied(requirement, copyText) {
			t.Fatalf("expected requirement %q to be satisfied", requirement)
		}
	}
}

func TestMandatoryElementSatisfiedStillWarnsForMissingVisibleRequirement(t *testing.T) {
	t.Parallel()
	copyText := strings.ToLower("新产品准备打样时，先核验判断依据。")

	if mandatoryElementSatisfied("低摩擦咨询动作", copyText) {
		t.Fatal("consultation requirement should be missing")
	}
	if mandatoryElementSatisfied("必须展示品牌授权标识", copyText) {
		t.Fatal("unknown visible requirement should still require a literal match")
	}
}

func testService() Service {
	sequence := 0
	return Service{
		Repository: &memoryRepository{
			intakes: map[string]CreativeIntake{}, tasks: map[string]TaskDetail{}, renders: map[string]RenderJob{},
			versions: map[string]CreativeVersion{}, packages: map[string]CreativePackage{},
		},
		Projects:                            testProjects{},
		AllowLegacyTaskStrategyIntakeWrites: true,
		Now:                                 func() time.Time { return time.Date(2026, time.July, 23, 1, 0, 0, 0, time.UTC) },
		NewID: func(prefix string) (string, error) {
			sequence++
			return fmt.Sprintf("%s_%d", prefix, sequence), nil
		},
	}
}

type testProjects struct{}

type strategyPackageReader struct {
	snapshot StrategyPackageSnapshot
}

type taskStrategyReader struct {
	snapshot TaskStrategySnapshot
	err      error
}

type requirementSnapshotReader struct {
	snapshot RequirementSnapshot
	err      error
}

type testAssetReader struct {
	snapshot  CreativeAssetSnapshot
	snapshots map[contract.AssetID]CreativeAssetSnapshot
	err       error
}

type testRenderScheduler struct{ render RenderJob }

func (s *testRenderScheduler) ScheduleRender(_ context.Context, render RenderJob) error {
	s.render = render
	return nil
}

type testVideoComposer struct{}

func (testVideoComposer) ComposePreRoll(context.Context, media.PreRollCompositionRequest) (media.CompositionOutput, error) {
	return media.CompositionOutput{
		Content: io.NopCloser(bytes.NewReader([]byte("rendered-video"))), SizeBytes: 14,
		Metadata: assets.VideoMetadata{DurationMS: 1000, WidthPixels: 720, HeightPixels: 1280, FrameRate: "25/1", VideoCodec: "h264"},
	}, nil
}

type testRenderedAssetWriter struct {
	ref         contract.ProjectAssetRef
	renderJobID string
}

func (w *testRenderedAssetWriter) IngestRenderedVideo(_ context.Context, _ contract.RequestContext, _ contract.ProjectID, renderJobID string, _ io.Reader, _ int64) (contract.ProjectAssetRef, error) {
	w.renderJobID = renderJobID
	return w.ref, nil
}

func (r testAssetReader) ReadForCreative(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, ref contract.AssetVersionRef) (CreativeAssetSnapshot, error) {
	if r.snapshots != nil {
		value, ok := r.snapshots[ref.AssetID]
		if !ok {
			return CreativeAssetSnapshot{}, ErrNotFound
		}
		return value, r.err
	}
	return r.snapshot, r.err
}

func (r strategyPackageReader) ReadForCreative(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, reference StrategyPackageReference) (StrategyPackageSnapshot, error) {
	value := r.snapshot
	if value.PackageID == "" {
		value.PackageID, value.PackageVersion, value.ContentHash = reference.PackageID, reference.PackageVersion, reference.ExpectedContentHash
	}
	if value.ContentHash != reference.ExpectedContentHash {
		return StrategyPackageSnapshot{}, fmt.Errorf("hash mismatch")
	}
	return value, nil
}

func (r taskStrategyReader) ReadTaskStrategyForCreative(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, reference TaskStrategyReference) (TaskStrategySnapshot, error) {
	if r.err != nil {
		return TaskStrategySnapshot{}, r.err
	}
	value := r.snapshot
	if value.PlanID == "" {
		value.PlanID, value.StrategyVersion, value.ContentHash = reference.PlanID, reference.StrategyVersion, reference.ExpectedContentHash
	}
	if value.PlanID != reference.PlanID || value.StrategyVersion != reference.StrategyVersion ||
		value.ContentHash != reference.ExpectedContentHash {
		return TaskStrategySnapshot{}, fmt.Errorf("task strategy reference mismatch")
	}
	return value, nil
}

func (r requirementSnapshotReader) ReadRequirementForCreative(_ context.Context, _ contract.ActorContext, _ contract.ProjectID, _ RequirementSnapshotReference, _ BusinessCapabilityReference) (RequirementSnapshot, error) {
	return r.snapshot, r.err
}

func (testProjects) RequireActiveContext(_ context.Context, actor contract.ActorContext, projectID contract.ProjectID) (contract.ProjectContext, error) {
	brand := contract.BrandID("brand_1")
	return contract.ProjectContext{OrganizationID: actor.OrganizationID, ProjectID: projectID, BrandID: &brand, ProductIDs: []contract.ProductID{}, ProjectContextVersion: 1}, nil
}

type memoryRepository struct {
	intakes  map[string]CreativeIntake
	tasks    map[string]TaskDetail
	renders  map[string]RenderJob
	versions map[string]CreativeVersion
	packages map[string]CreativePackage
}

func (r *memoryRepository) CreateIntake(_ context.Context, intake CreativeIntake) (CreativeIntake, bool, error) {
	for _, existing := range r.intakes {
		if existing.IdempotencyKey == intake.IdempotencyKey && existing.Principal == intake.Principal && existing.ProjectID == intake.ProjectID {
			if existing.RequestHash != intake.RequestHash {
				return CreativeIntake{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
		if intake.Source == IntakeSourceStrategyPackage && existing.Source == IntakeSourceStrategyPackage &&
			sameStrategyPackage(existing.Request.StrategyPackage, intake.Request.StrategyPackage) &&
			existing.Request.SelectedRouteID == intake.Request.SelectedRouteID &&
			sameTaskOverlay(existing.Request.TaskOverlay, intake.Request.TaskOverlay) {
			return existing, true, nil
		}
		if intake.Source == IntakeSourceTaskStrategy && existing.Source == IntakeSourceTaskStrategy &&
			sameTaskStrategy(existing.Request.TaskStrategy, intake.Request.TaskStrategy) {
			return existing, true, nil
		}
		if intake.Source == IntakeSourceRequirement && existing.Source == IntakeSourceRequirement &&
			intake.InputIdentityHash != "" && existing.InputIdentityHash == intake.InputIdentityHash {
			return existing, true, nil
		}
	}
	r.intakes[intake.ID] = intake
	return intake, false, nil
}

func (r *memoryRepository) UpdateIntakeReadiness(
	_ context.Context,
	organizationID contract.OrganizationID,
	projectID contract.ProjectID,
	intakeID string,
	expectedVersion int64,
	status IntakeStatus,
	missingFields []string,
	confirmedBy string,
	updatedAt time.Time,
) (CreativeIntake, error) {
	intake, ok := r.intakes[intakeID]
	if !ok || intake.OrganizationID != organizationID || intake.ProjectID != projectID {
		return CreativeIntake{}, ErrNotFound
	}
	if intake.Version != expectedVersion {
		return CreativeIntake{}, ErrVersionConflict
	}
	intake.Status = status
	intake.MissingFields = append([]string{}, missingFields...)
	intake.ConfirmedBy = confirmedBy
	intake.Version++
	intake.UpdatedAt = updatedAt
	r.intakes[intakeID] = intake
	return intake, nil
}

func sameTaskOverlay(left, right *TaskOverlayReference) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.OverlayID == right.OverlayID &&
		left.ExpectedContentHash == right.ExpectedContentHash
}

func sameTaskStrategy(left, right *TaskStrategyReference) bool {
	return left != nil && right != nil && left.PlanID == right.PlanID &&
		left.StrategyVersion == right.StrategyVersion &&
		left.ExpectedContentHash == right.ExpectedContentHash
}

func sameStrategyPackage(left, right *StrategyPackageReference) bool {
	return left != nil && right != nil && left.PackageID == right.PackageID && left.PackageVersion == right.PackageVersion && left.ExpectedContentHash == right.ExpectedContentHash
}
func (r *memoryRepository) ListIntakes(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ int) ([]CreativeIntake, error) {
	values := make([]CreativeIntake, 0, len(r.intakes))
	for _, value := range r.intakes {
		values = append(values, value)
	}
	return values, nil
}
func (r *memoryRepository) GetIntake(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string) (CreativeIntake, error) {
	value, ok := r.intakes[id]
	if !ok {
		return CreativeIntake{}, ErrNotFound
	}
	return value, nil
}

func (r *memoryRepository) CreateTask(_ context.Context, task CreativeTask, draft ImageTextDraft) (CreativeTask, error) {
	r.tasks[task.ID] = TaskDetail{Task: task, Intake: r.intakes[task.IntakeID], Draft: draft, ProductionJobs: []ProductionJob{}}
	return task, nil
}
func (r *memoryRepository) CreateVideoTask(_ context.Context, task CreativeTask, draft VideoDraft) (CreativeTask, error) {
	value := draft
	r.tasks[task.ID] = TaskDetail{Task: task, Intake: r.intakes[task.IntakeID], VideoDraft: &value, ProductionJobs: []ProductionJob{}}
	return task, nil
}
func (r *memoryRepository) CreateRenderJob(_ context.Context, value RenderJob) (RenderJob, bool, error) {
	for _, existing := range r.renders {
		if existing.IdempotencyKey == value.IdempotencyKey && existing.CreatedBy == value.CreatedBy && existing.ProjectID == value.ProjectID {
			if existing.RequestHash != value.RequestHash {
				return RenderJob{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
	}
	r.renders[value.ID] = value
	return value, false, nil
}
func (r *memoryRepository) GetRenderJob(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string) (RenderJob, error) {
	value, ok := r.renders[id]
	if !ok {
		return RenderJob{}, ErrNotFound
	}
	return value, nil
}
func (r *memoryRepository) MarkRenderRunning(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string, now time.Time) (RenderJob, error) {
	value := r.renders[id]
	value.Status, value.UpdatedAt = RenderRunning, now
	r.renders[id] = value
	return value, nil
}
func (r *memoryRepository) CompleteRenderJob(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string, ref contract.ProjectAssetRef, now time.Time) error {
	value := r.renders[id]
	value.Status, value.OutputAsset, value.UpdatedAt = RenderSucceeded, &ref, now
	r.renders[id] = value
	return nil
}
func (r *memoryRepository) FailRenderJob(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id, code, message string, now time.Time) error {
	value := r.renders[id]
	value.Status, value.ErrorCode, value.ErrorMessage, value.UpdatedAt = RenderFailed, code, message, now
	r.renders[id] = value
	return nil
}
func (r *memoryRepository) ListTasks(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, _ int) ([]CreativeTask, error) {
	values := make([]CreativeTask, 0, len(r.tasks))
	for _, value := range r.tasks {
		if value.Task.Status == TaskArchived {
			continue
		}
		values = append(values, value.Task)
	}
	return values, nil
}
func (r *memoryRepository) RenameTask(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, taskID string, expectedVersion int64, displayName string, now time.Time) (CreativeTask, error) {
	value, ok := r.tasks[taskID]
	if !ok {
		return CreativeTask{}, ErrNotFound
	}
	if value.Task.Version != expectedVersion {
		return CreativeTask{}, ErrVersionConflict
	}
	value.Task.DisplayName = displayName
	value.Task.Version++
	value.Task.UpdatedAt = now
	r.tasks[taskID] = value
	return value.Task, nil
}
func (r *memoryRepository) GetTaskDetail(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string) (TaskDetail, error) {
	value, ok := r.tasks[id]
	if !ok {
		return TaskDetail{}, ErrNotFound
	}
	return value, nil
}
func (r *memoryRepository) CreateShortDramaGenerationAttempt(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, attempt ShortDramaGenerationAttempt) (ShortDramaGenerationAttempt, error) {
	value, ok := r.tasks[attempt.TaskID]
	if !ok {
		return ShortDramaGenerationAttempt{}, ErrNotFound
	}
	for _, existing := range value.ShortDramaGenerationAttempts {
		if existing.ProviderJobID == attempt.ProviderJobID {
			return existing, nil
		}
	}
	value.ShortDramaGenerationAttempts = append(value.ShortDramaGenerationAttempts, attempt)
	r.tasks[attempt.TaskID] = value
	return attempt, nil
}
func (r *memoryRepository) CreateGamePrerollGenerationAttempt(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, attempt GamePrerollGenerationAttempt) (GamePrerollGenerationAttempt, error) {
	value, ok := r.tasks[attempt.TaskID]
	if !ok {
		return GamePrerollGenerationAttempt{}, ErrNotFound
	}
	for _, existing := range value.GamePrerollGenerationAttempts {
		if existing.ProviderJobID == attempt.ProviderJobID {
			return existing, nil
		}
	}
	value.GamePrerollGenerationAttempts = append(value.GamePrerollGenerationAttempts, attempt)
	r.tasks[attempt.TaskID] = value
	return attempt, nil
}
func (r *memoryRepository) ArchiveTask(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, taskID string, now time.Time) error {
	value, ok := r.tasks[taskID]
	if !ok {
		return ErrNotFound
	}
	if value.Task.Status == TaskArchived {
		return ErrInvalidState
	}
	value.Task.Status = TaskArchived
	value.Task.UpdatedAt = now
	r.tasks[taskID] = value
	return nil
}
func (r *memoryRepository) ReviseDraft(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, taskID string, expectedVersion int64, draft ImageTextDraft) (ImageTextDraft, error) {
	value, ok := r.tasks[taskID]
	if !ok {
		return ImageTextDraft{}, ErrNotFound
	}
	if value.Draft.Version != expectedVersion || value.Task.Version != expectedVersion {
		return ImageTextDraft{}, ErrVersionConflict
	}
	value.Draft = draft
	value.Task.Version = draft.Version
	value.Task.UpdatedAt = draft.CreatedAt
	r.tasks[taskID] = value
	return draft, nil
}
func (r *memoryRepository) ReviseVideoDraft(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, taskID string, expectedRevision int64, draft VideoDraft, status TaskStatus) (VideoDraft, error) {
	value, ok := r.tasks[taskID]
	if !ok {
		return VideoDraft{}, ErrNotFound
	}
	if value.VideoDraft == nil || value.VideoDraft.Revision != expectedRevision || draft.Revision != expectedRevision+1 {
		return VideoDraft{}, ErrVersionConflict
	}
	value.VideoDraft = &draft
	value.Task.Status = status
	value.Task.Version++
	value.Task.UpdatedAt = draft.CreatedAt
	r.tasks[taskID] = value
	return draft, nil
}
func (r *memoryRepository) RegisterProductionJob(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, taskID string, job ProductionJob) error {
	value, ok := r.tasks[taskID]
	if !ok {
		return ErrNotFound
	}
	for _, existing := range value.ProductionJobs {
		if existing.Kind == job.Kind {
			if existing.ProviderJobID == job.ProviderJobID {
				return nil
			}
			return ErrProviderJobConflict
		}
	}
	value.ProductionJobs = append(value.ProductionJobs, job)
	r.tasks[taskID] = value
	return nil
}

func (r *memoryRepository) CreateVersion(_ context.Context, value CreativeVersion) (CreativeVersion, bool, error) {
	for _, existing := range r.versions {
		if existing.ProjectID == value.ProjectID && existing.CreatedBy == value.CreatedBy && existing.IdempotencyKey == value.IdempotencyKey {
			if existing.RequestHash != value.RequestHash {
				return CreativeVersion{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
		if ((value.TaskID != "" && existing.TaskID == value.TaskID) || (value.EditTaskID != "" && existing.EditTaskID == value.EditTaskID)) && existing.Version == value.Version {
			if !existing.ContentHash.Equal(value.ContentHash) {
				return CreativeVersion{}, false, ErrVersionConflict
			}
			return existing, false, nil
		}
	}
	r.versions[value.ID] = value
	return value, false, nil
}

func (r *memoryRepository) GetVersion(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string) (CreativeVersion, error) {
	value, ok := r.versions[id]
	if !ok {
		return CreativeVersion{}, ErrNotFound
	}
	return value, nil
}

func (r *memoryRepository) ListVersions(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, taskID string, limit int) ([]CreativeVersion, error) {
	values := make([]CreativeVersion, 0, len(r.versions))
	for _, value := range r.versions {
		if value.OrganizationID == organizationID && value.ProjectID == projectID && (taskID == "" || value.TaskID == taskID || value.EditTaskID == taskID) {
			values = append(values, value)
			if len(values) == limit {
				break
			}
		}
	}
	return values, nil
}

func (r *memoryRepository) RecordVersionCheck(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string, check CreativeCheck) (CreativeVersion, error) {
	value, ok := r.versions[id]
	if !ok {
		return CreativeVersion{}, ErrNotFound
	}
	value.Status = CreativeVersionChecked
	value.Check = &check
	r.versions[id] = value
	return value, nil
}

func (r *memoryRepository) ApproveVersion(_ context.Context, _ contract.OrganizationID, _ contract.ProjectID, id string, approval CreativeApproval) (CreativeVersion, error) {
	value, ok := r.versions[id]
	if !ok {
		return CreativeVersion{}, ErrNotFound
	}
	if value.Status != CreativeVersionChecked || value.Check == nil || !value.Check.Passed {
		return CreativeVersion{}, ErrInvalidState
	}
	value.Status = CreativeVersionApproved
	value.Approval = &approval
	r.versions[id] = value
	return value, nil
}

func (r *memoryRepository) CreatePackage(_ context.Context, value CreativePackage) (CreativePackage, error) {
	for _, existing := range r.packages {
		if existing.CreativeVersionID == value.CreativeVersionID {
			return existing, nil
		}
	}
	r.packages[value.ID] = value
	return value, nil
}

func (r *memoryRepository) ListPackages(_ context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, limit int) ([]CreativePackage, error) {
	values := make([]CreativePackage, 0, len(r.packages))
	for _, value := range r.packages {
		if value.OrganizationID == organizationID && value.ProjectID == projectID {
			values = append(values, value)
			if len(values) == limit {
				break
			}
		}
	}
	return values, nil
}

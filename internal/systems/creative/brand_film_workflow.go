package creative

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	fixturefiles "github.com/shikanon/cookies/api/fixtures"
	"github.com/shikanon/cookies/internal/platform/contract"
)

const guerlainBrandFixtureFile = "creative-video-intake-brand-video-guerlain-v1.json"

type UpdateBrandBriefAnalysisRequest struct {
	ExpectedRevision int64                     `json:"expected_revision"`
	Analysis         BrandBriefAnalysisVersion `json:"analysis"`
}

type BrandFilmRevisionRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type SelectBrandConceptRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	ConceptID        string `json:"concept_id"`
}

type UpdateBrandConceptsRequest struct {
	ExpectedRevision int64                  `json:"expected_revision"`
	Candidates       []BrandCreativeConcept `json:"candidates"`
}

type UpdateBrandFilmPlanRequest struct {
	ExpectedRevision int64                `json:"expected_revision"`
	Plan             BrandFilmPlanVersion `json:"plan"`
}

type brandFixtureDocument struct {
	ContractVersion string `json:"contract_version"`
	Source          struct {
		FixtureID string `json:"fixture_id"`
		Documents []struct {
			FileName string   `json:"file_name"`
			Locator  string   `json:"locator"`
			SHA256   string   `json:"sha256"`
			PageRefs []string `json:"page_refs"`
		} `json:"documents"`
	} `json:"source"`
	Campaign struct {
		Objective   string   `json:"objective"`
		Audience    string   `json:"audience"`
		CoreMessage string   `json:"core_message"`
		CTA         string   `json:"call_to_action"`
		Channels    []string `json:"channels"`
	} `json:"campaign"`
	Video struct {
		Mode        string `json:"mode"`
		Purpose     string `json:"purpose"`
		Duration    int    `json:"target_duration_seconds"`
		AspectRatio string `json:"aspect_ratio"`
	} `json:"video"`
	Product struct {
		BrandName      string   `json:"brand_name"`
		ProductName    string   `json:"product_name"`
		SellingPoints  []string `json:"selling_points"`
		ProofPoints    []string `json:"proof_points"`
		UsageScenarios []string `json:"usage_scenarios"`
	} `json:"product"`
	Creative struct {
		Concept    string   `json:"concept"`
		Tone       []string `json:"tone"`
		Visual     []string `json:"visual_keywords"`
		Mandatory  []string `json:"mandatory_elements"`
		Prohibited []string `json:"prohibited_claims"`
	} `json:"creative"`
	Evidence []struct {
		Locator string `json:"locator"`
	} `json:"evidence_refs"`
}

func readGuerlainBrandFixture() (brandFixtureDocument, string, []byte, error) {
	raw, err := fixturefiles.Files.ReadFile(guerlainBrandFixtureFile)
	if err != nil {
		return brandFixtureDocument{}, "", nil, fmt.Errorf("read brand film fixture: %w", err)
	}
	var value brandFixtureDocument
	if err := json.Unmarshal(raw, &value); err != nil {
		return brandFixtureDocument{}, "", nil, fmt.Errorf("decode brand film fixture: %w", err)
	}
	if value.ContractVersion != "creative-video-intake/v1" || value.Source.FixtureID != GuerlainBrandFixtureID ||
		value.Video.Mode != PerformanceModeBrandFilm || value.Video.Purpose != "brand" ||
		value.Video.Duration != 15 || value.Video.AspectRatio != "9:16" || len(value.Source.Documents) != 1 {
		return brandFixtureDocument{}, "", nil, fmt.Errorf("brand film fixture contract is invalid")
	}
	hash, err := contract.CanonicalJSONHash(value)
	if err != nil {
		return brandFixtureDocument{}, "", nil, fmt.Errorf("hash brand film fixture: %w", err)
	}
	return value, "sha256:" + hash, raw, nil
}

func (s Service) EnsureBrandFilmFixtureWorkspace(ctx context.Context, requestContext contract.RequestContext, projectID contract.ProjectID, key contract.IdempotencyKey) (TaskDetail, error) {
	if existing, err := s.GetLatestBrandFilmWorkspace(ctx, requestContext.Actor, projectID); err == nil {
		return existing, nil
	} else if err != ErrNotFound {
		return TaskDetail{}, err
	}
	fixture, fixtureHash, raw, err := readGuerlainBrandFixture()
	if err != nil {
		return TaskDetail{}, err
	}
	document := fixture.Source.Documents[0]
	intake, err := s.CreateIntake(ctx, requestContext, projectID, key, CreateIntakeRequest{
		Source: IntakeSourceManual, Format: FormatVideo, PerformanceMode: PerformanceModeBrandFilm,
		Channel: ChannelDouyin, Objective: fixture.Campaign.Objective, Audience: fixture.Campaign.Audience,
		CoreMessage: fixture.Campaign.CoreMessage, CallToAction: fixture.Campaign.CTA, Concept: fixture.Creative.Concept,
		Tone: append([]string{}, fixture.Creative.Tone...), VisualKeywords: append([]string{}, fixture.Creative.Visual...),
		Mandatory: append([]string{}, fixture.Creative.Mandatory...), Prohibited: append([]string{}, fixture.Creative.Prohibited...),
		CreativeRoutes: []CreativeRouteSnapshot{{
			RouteID: ManualBrandFilmRouteID, RouteType: PerformanceModeBrandFilm, VideoPurpose: "brand",
			Channels: []string{"douyin"}, Reason: "娇兰 25X 蜂皇水 15 秒品牌广告固定开发样例",
			TargetDurationSeconds: 15, AspectRatio: "9:16", EvidenceRefs: append([]string{}, document.PageRefs...),
			RequiresHumanConfirmation: true,
		}},
		ManualBrandFilm: &ManualBrandFilmInput{
			FixtureID: GuerlainBrandFixtureID, FixtureVersion: 1, FixtureHash: fixtureHash,
			BriefName: document.FileName, BriefText: string(raw), ProductName: fixture.Product.ProductName,
		},
	})
	if err != nil {
		return TaskDetail{}, err
	}
	if existing, getErr := s.taskForIntake(ctx, requestContext.Actor, projectID, intake.ID); getErr == nil {
		return existing, nil
	} else if getErr != ErrNotFound {
		return TaskDetail{}, getErr
	}
	task, err := s.CreateVideoTask(ctx, requestContext.Actor, projectID, intake.ID, CreateVideoTaskRequest{
		SelectedRouteID: ManualBrandFilmRouteID, Channel: ChannelDouyin, ConfirmRoute: true,
		Concept: fixture.Creative.Concept, Prompt: "等待确认的品牌广告生成计划",
		CallToAction: fixture.Campaign.CTA, Mandatory: fixture.Creative.Mandatory, Prohibited: fixture.Creative.Prohibited,
	})
	if err != nil {
		return TaskDetail{}, err
	}
	return s.GetBrandFilmWorkspace(ctx, requestContext.Actor, projectID, task.ID)
}

func (s Service) taskForIntake(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, intakeID string) (TaskDetail, error) {
	tasks, err := s.Repository.ListTasks(ctx, actor.OrganizationID, projectID, 100)
	if err != nil {
		return TaskDetail{}, err
	}
	for _, task := range tasks {
		if task.IntakeID == intakeID && task.PerformanceMode == PerformanceModeBrandFilm && task.Status != TaskArchived {
			return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, task.ID)
		}
	}
	return TaskDetail{}, ErrNotFound
}

func (s Service) GetLatestBrandFilmWorkspace(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID) (TaskDetail, error) {
	if s.Repository == nil || s.Projects == nil {
		return TaskDetail{}, fmt.Errorf("creative brand film dependencies are incomplete")
	}
	if !actor.HasScope(ScopeRead) {
		return TaskDetail{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return TaskDetail{}, err
	}
	tasks, err := s.Repository.ListTasks(ctx, actor.OrganizationID, projectID, 100)
	if err != nil {
		return TaskDetail{}, err
	}
	for _, task := range tasks {
		if task.Format == FormatVideo && task.PerformanceMode == PerformanceModeBrandFilm && task.Status != TaskArchived {
			return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, task.ID)
		}
	}
	return TaskDetail{}, ErrNotFound
}

func (s Service) GetBrandFilmWorkspace(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string) (TaskDetail, error) {
	return s.requireBrandFilmWorkspace(ctx, actor, projectID, taskID, false)
}

func (s Service) AnalyzeBrandFilmBrief(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BrandFilmRevisionRequest) (TaskDetail, error) {
	detail, project, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	brand := detail.VideoDraft.BrandFilm
	planner := s.BrandFilmPlanner
	if planner == nil {
		planner = DeterministicBrandFilmPlanner{}
	}
	analysis, err := planner.AnalyzeBrief(ctx, actor, project, brand.SourceSnapshot, int64(len(brand.BriefAnalyses)+1), s.now())
	if err != nil {
		return TaskDetail{}, err
	}
	analysis.AssetCandidates = reconcileBriefProductAssets(brand.SourceSnapshot.BriefText, analysis.AssetCandidates)
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	next.BrandFilm.Revision = next.Revision
	next.BrandFilm.Stage = BrandFilmBriefDraft
	next.BrandFilm.BriefAnalyses = append(next.BrandFilm.BriefAnalyses, analysis)
	next.BrandFilm.ConceptSets, next.BrandFilm.FilmPlans, next.BrandFilm.SelectedConceptID = nil, nil, ""
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.Readiness = CreativeReadiness{PlanningReady: false, GenerationReady: false, ProductionReady: false, Blockers: []string{"brief_analysis_confirmation"}}
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) UpdateBrandFilmBrief(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request UpdateBrandBriefAnalysisRequest) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	current := detail.VideoDraft.BrandFilm.CurrentAnalysis()
	if request.ExpectedRevision != detail.VideoDraft.Revision || current == nil {
		return TaskDetail{}, ErrVersionConflict
	}
	value := request.Analysis
	value.Revision, value.Confirmed, value.ConfirmedBy, value.ConfirmedAt = current.Revision+1, false, "", nil
	value.ModelAlias, value.ModelVersion, value.RouteRevisionID, value.PromptVersion = current.ModelAlias, current.ModelVersion, current.RouteRevisionID, current.PromptVersion
	value.CreatedAt = s.now()
	if err := value.Validate(); err != nil {
		return TaskDetail{}, err
	}
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmBriefDraft
	next.BrandFilm.BriefAnalyses = append(next.BrandFilm.BriefAnalyses, value)
	next.BrandFilm.ConceptSets, next.BrandFilm.FilmPlans, next.BrandFilm.SelectedConceptID = nil, nil, ""
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.Readiness = CreativeReadiness{PlanningReady: false, GenerationReady: false, ProductionReady: false, Blockers: []string{"brief_analysis_confirmation"}}
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) ConfirmBrandFilmBrief(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BrandFilmRevisionRequest) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	current := detail.VideoDraft.BrandFilm.CurrentAnalysis()
	if request.ExpectedRevision != detail.VideoDraft.Revision || current == nil {
		return TaskDetail{}, ErrVersionConflict
	}
	requiredAssets := map[string]bool{"product_front": false, "logo": false}
	for _, asset := range current.AssetCandidates {
		if _, required := requiredAssets[asset.Role]; required && asset.UserConfirmed {
			requiredAssets[asset.Role] = true
		}
	}
	if !requiredAssets["product_front"] || !requiredAssets["logo"] {
		return TaskDetail{}, fmt.Errorf("confirm the product_front and logo assets before confirming the Brief")
	}
	now := s.now()
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	confirmed := next.BrandFilm.BriefAnalyses[len(next.BrandFilm.BriefAnalyses)-1]
	confirmed.Confirmed, confirmed.ConfirmedBy, confirmed.ConfirmedAt = true, actor.Principal.ID, &now
	next.BrandFilm.BriefAnalyses[len(next.BrandFilm.BriefAnalyses)-1] = confirmed
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmBriefConfirmed
	next.BrandFilm.Readiness = CreativeReadiness{PlanningReady: true, GenerationReady: false, ProductionReady: false, Blockers: []string{"creative_concept_selection", "production_plan_confirmation", "prompt_package"}}
	next.BrandFilm.UpdatedAt = now
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) GenerateBrandFilmConcepts(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BrandFilmRevisionRequest) (TaskDetail, error) {
	detail, project, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	analysis := detail.VideoDraft.BrandFilm.CurrentAnalysis()
	if request.ExpectedRevision != detail.VideoDraft.Revision {
		return TaskDetail{}, ErrVersionConflict
	}
	if analysis == nil || !analysis.Confirmed {
		return TaskDetail{}, ErrInvalidState
	}
	planner := s.BrandFilmPlanner
	if planner == nil {
		planner = DeterministicBrandFilmPlanner{}
	}
	concepts, err := planner.GenerateConcepts(ctx, actor, project, detail.VideoDraft.BrandFilm.SourceSnapshot, *analysis, int64(len(detail.VideoDraft.BrandFilm.ConceptSets)+1), s.now())
	if err != nil {
		return TaskDetail{}, err
	}
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmConceptSelection
	next.BrandFilm.ConceptSets = append(next.BrandFilm.ConceptSets, concepts)
	next.BrandFilm.SelectedConceptID, next.BrandFilm.FilmPlans = "", nil
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) UpdateBrandFilmConcepts(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request UpdateBrandConceptsRequest) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	current := detail.VideoDraft.BrandFilm.CurrentConceptSet()
	if request.ExpectedRevision != detail.VideoDraft.Revision || current == nil {
		return TaskDetail{}, ErrVersionConflict
	}
	value := *current
	value.Revision = current.Revision + 1
	value.Candidates = append([]BrandCreativeConcept{}, request.Candidates...)
	value.CreatedAt = s.now()
	for index := range value.Candidates {
		value.Candidates[index].Selected = false
		value.Candidates[index].Confirmed = false
	}
	if err := value.Validate(); err != nil {
		return TaskDetail{}, err
	}
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmConceptSelection
	next.BrandFilm.ConceptSets = append(next.BrandFilm.ConceptSets, value)
	next.BrandFilm.SelectedConceptID, next.BrandFilm.FilmPlans = "", nil
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.Readiness = CreativeReadiness{PlanningReady: true, GenerationReady: false, ProductionReady: false, Blockers: []string{"creative_concept_selection", "production_plan_confirmation", "prompt_package"}}
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) SelectBrandFilmConcept(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request SelectBrandConceptRequest) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	set := detail.VideoDraft.BrandFilm.CurrentConceptSet()
	if request.ExpectedRevision != detail.VideoDraft.Revision || set == nil || strings.TrimSpace(request.ConceptID) == "" {
		return TaskDetail{}, ErrVersionConflict
	}
	found := false
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	currentSet := &next.BrandFilm.ConceptSets[len(next.BrandFilm.ConceptSets)-1]
	for index := range currentSet.Candidates {
		selected := currentSet.Candidates[index].ID == request.ConceptID
		currentSet.Candidates[index].Selected, currentSet.Candidates[index].Confirmed = selected, selected
		found = found || selected
	}
	if !found {
		return TaskDetail{}, ErrNotFound
	}
	next.Revision++
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmConceptConfirmed
	next.BrandFilm.SelectedConceptID, next.BrandFilm.FilmPlans = request.ConceptID, nil
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) GenerateBrandFilmPlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BrandFilmRevisionRequest) (TaskDetail, error) {
	detail, project, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	brand := detail.VideoDraft.BrandFilm
	analysis, concepts := brand.CurrentAnalysis(), brand.CurrentConceptSet()
	if request.ExpectedRevision != detail.VideoDraft.Revision || analysis == nil || concepts == nil || brand.SelectedConceptID == "" {
		return TaskDetail{}, ErrInvalidState
	}
	var selected *BrandCreativeConcept
	for index := range concepts.Candidates {
		if concepts.Candidates[index].ID == brand.SelectedConceptID && concepts.Candidates[index].Confirmed {
			selected = &concepts.Candidates[index]
		}
	}
	if selected == nil {
		return TaskDetail{}, ErrInvalidState
	}
	planner := s.BrandFilmPlanner
	if planner == nil {
		planner = DeterministicBrandFilmPlanner{}
	}
	plan, err := planner.GenerateFilmPlan(ctx, actor, project, brand.SourceSnapshot, *analysis, *selected, int64(len(brand.FilmPlans)+1), s.now())
	if err != nil {
		return TaskDetail{}, err
	}
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmPlanDraft
	next.BrandFilm.FilmPlans = append(next.BrandFilm.FilmPlans, plan)
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) UpdateBrandFilmPlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request UpdateBrandFilmPlanRequest) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	current := detail.VideoDraft.BrandFilm.CurrentPlan()
	if request.ExpectedRevision != detail.VideoDraft.Revision || current == nil {
		return TaskDetail{}, ErrVersionConflict
	}
	value := request.Plan
	value.Revision, value.ConceptID, value.Confirmed, value.ConfirmedBy, value.ConfirmedAt = current.Revision+1, current.ConceptID, false, "", nil
	value.ModelAlias, value.ModelVersion, value.RouteRevisionID, value.PromptVersion = current.ModelAlias, current.ModelVersion, current.RouteRevisionID, current.PromptVersion
	value.CreatedAt = s.now()
	if err := value.Validate(); err != nil {
		return TaskDetail{}, err
	}
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmPlanDraft
	next.BrandFilm.FilmPlans = append(next.BrandFilm.FilmPlans, value)
	next.BrandFilm.Generation, next.BrandFilm.Audio, next.BrandFilm.QualityRuns, next.BrandFilm.Delivery = nil, nil, nil, nil
	next.BrandFilm.Readiness = CreativeReadiness{PlanningReady: true, GenerationReady: false, ProductionReady: false, Blockers: []string{"production_plan_confirmation", "prompt_package", "generation_confirmation"}}
	next.BrandFilm.UpdatedAt = s.now()
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) ConfirmBrandFilmPlan(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request BrandFilmRevisionRequest) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, true)
	if err != nil {
		return TaskDetail{}, err
	}
	current := detail.VideoDraft.BrandFilm.CurrentPlan()
	if request.ExpectedRevision != detail.VideoDraft.Revision || current == nil {
		return TaskDetail{}, ErrVersionConflict
	}
	now := s.now()
	next := cloneBrandVideoDraft(*detail.VideoDraft)
	next.Revision++
	confirmed := next.BrandFilm.FilmPlans[len(next.BrandFilm.FilmPlans)-1]
	confirmed.Confirmed, confirmed.ConfirmedBy, confirmed.ConfirmedAt = true, actor.Principal.ID, &now
	next.BrandFilm.FilmPlans[len(next.BrandFilm.FilmPlans)-1] = confirmed
	next.BrandFilm.Revision, next.BrandFilm.Stage = next.Revision, BrandFilmPlanConfirmed
	next.BrandFilm.Readiness = CreativeReadiness{PlanningReady: true, GenerationReady: false, ProductionReady: false, Blockers: []string{"reference_assets", "prompt_package", "generation_confirmation"}}
	next.BrandFilm.UpdatedAt = now
	return s.persistBrandFilmDraft(ctx, actor, projectID, taskID, *detail.VideoDraft, next)
}

func (s Service) persistBrandFilmDraft(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, previous VideoDraft, next VideoDraft) (TaskDetail, error) {
	if _, err := s.ViralRemakes.ReviseVideoDraft(ctx, actor.OrganizationID, projectID, taskID, previous.Revision, next, TaskInProgress); err != nil {
		return TaskDetail{}, err
	}
	return s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
}

func (s Service) requireBrandFilmWorkspace(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, write bool) (TaskDetail, error) {
	detail, _, err := s.requireBrandFilmWorkspaceWithProject(ctx, actor, projectID, taskID, write)
	return detail, err
}

func (s Service) requireBrandFilmWorkspaceWithProject(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, write bool) (TaskDetail, contract.ProjectContext, error) {
	if s.Repository == nil || s.ViralRemakes == nil || s.Projects == nil {
		return TaskDetail{}, contract.ProjectContext{}, fmt.Errorf("creative brand film dependencies are incomplete")
	}
	required := ScopeRead
	if write {
		required = ScopeWrite
	}
	if !actor.HasScope(required) {
		return TaskDetail{}, contract.ProjectContext{}, fmt.Errorf("%s scope is required", required)
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, contract.ProjectContext{}, err
	}
	detail, err := s.Repository.GetTaskDetail(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return TaskDetail{}, contract.ProjectContext{}, err
	}
	if detail.Task.Format != FormatVideo || detail.Task.PerformanceMode != PerformanceModeBrandFilm || detail.VideoDraft == nil || detail.VideoDraft.BrandFilm == nil || detail.Task.Status == TaskArchived {
		return TaskDetail{}, contract.ProjectContext{}, ErrInvalidState
	}
	return detail, project, nil
}

func cloneBrandVideoDraft(value VideoDraft) VideoDraft {
	raw, _ := json.Marshal(value)
	var result VideoDraft
	_ = json.Unmarshal(raw, &result)
	return result
}

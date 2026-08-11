// Package httpserver exposes only platform-owned HTTP endpoints. It keeps
// transport concerns (request IDs, trace extraction, JSON errors, and
// authentication) out of future provider, knowledge, and workflow modules.
package httpserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/agent"
	"github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/identity"
	"github.com/shikanon/cookies/internal/platform/knowledge"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/platform/remix"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type Server struct {
	resolver          identity.Resolver
	projectAuthorizer identity.ProjectAuthorizer
	providerJobs      ProviderJobs
	readiness         ReadinessChecker
	identities        CurrentIdentityReader
	accounts          AccountManager
	projects          ProjectManager
	projectMembers    ProjectMembershipManager
	uploads           AssetUploadManager
	intakes           GeneratedIntakeManager
	creative          CreativeManager
	productionCenter  creative.ProductionCenterQuery
	productionAssets  creative.ProductionAssetQuery
	productionRetry   creative.ProductionRetryCommand
	sessions          SessionManager
	knowledge         KnowledgeManager
	remixPlans        RemixPlanManager
	evals             EvalManager
	agentRuns         AgentRunManager
	providerConfig    ProviderConfigurationReader
	mux               *http.ServeMux
	newID             func() (string, error)
}

type ReadinessChecker interface {
	Check(context.Context) error
}

type Dependencies struct {
	Resolver          identity.Resolver
	ProjectAuthorizer identity.ProjectAuthorizer
	ProviderJobs      ProviderJobs
	Readiness         ReadinessChecker
	Identities        CurrentIdentityReader
	Accounts          AccountManager
	Projects          ProjectManager
	ProjectMembers    ProjectMembershipManager
	Uploads           AssetUploadManager
	Intakes           GeneratedIntakeManager
	Creative          CreativeManager
	ProductionCenter  creative.ProductionCenterQuery
	ProductionAssets  creative.ProductionAssetQuery
	ProductionRetry   creative.ProductionRetryCommand
	Sessions          SessionManager
	Knowledge         KnowledgeManager
	RemixPlans        RemixPlanManager
	Evals             EvalManager
	AgentRuns         AgentRunManager
	ProviderConfig    ProviderConfigurationReader
	// AuthenticatedDomainMounts allow vertical systems to share the platform
	// listener and identity context without making this package import them.
	// Mount handlers remain responsible for project authorization and scopes.
	AuthenticatedDomainMounts []DomainMount
}

type DomainMount struct {
	Pattern string
	Handler http.Handler
}

type CurrentIdentityReader interface {
	GetCurrent(context.Context, contract.ActorContext) (identity.CurrentIdentity, error)
}
type AccountManager interface {
	ListOrganizations(context.Context, contract.ActorContext) ([]identity.OrganizationAccess, error)
	UpdateCurrentUser(context.Context, contract.ActorContext, string) (identity.User, error)
	ListOrganizationMembers(context.Context, contract.ActorContext) ([]identity.OrganizationMember, error)
	AddOrganizationMember(context.Context, contract.ActorContext, string, string) (identity.OrganizationMember, error)
	UpdateOrganizationMember(context.Context, contract.ActorContext, string, identity.UpdateOrganizationMembershipRequest) (identity.OrganizationMember, error)
}
type SessionManager interface {
	Login(context.Context, string, string) (identity.LoginResult, error)
	Logout(context.Context, string) error
	SwitchOrganization(context.Context, string, contract.OrganizationID) (identity.LoginResult, error)
	Cookie(string, time.Time) *http.Cookie
	ExpiredCookie() *http.Cookie
}
type ProjectManager interface {
	CreateBrand(context.Context, contract.ActorContext, string) (project.Brand, error)
	CreateProject(context.Context, contract.ActorContext, project.CreateProjectRequest) (project.Project, error)
	UpdateProject(context.Context, contract.ActorContext, contract.ProjectID, project.UpdateProjectRequest) (project.Project, error)
	GetDetail(context.Context, contract.ActorContext, contract.ProjectID) (project.ProjectDetail, error)
	CreateProjectArtifact(context.Context, contract.ActorContext, contract.ProjectID, project.CreateProjectArtifactRequest) (project.ProjectArtifact, error)
	ListProjectArtifacts(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ProjectArtifact, error)
	GetProjectArtifact(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ProjectArtifact, error)
	UpdateProjectArtifact(context.Context, contract.ActorContext, contract.ProjectID, string, project.UpdateProjectArtifactRequest) (project.ProjectArtifact, error)
	GetWorkbench(context.Context, contract.ActorContext, contract.ProjectID) (project.Workbench, error)
	RunWorkbenchQualityCheck(context.Context, contract.ActorContext, contract.ProjectID, project.RunWorkbenchQualityCheckRequest) (project.WorkbenchQualityCheckRun, error)
	RecordWorkbenchMaterialConfirmation(context.Context, contract.ActorContext, contract.ProjectID, project.RecordWorkbenchMaterialConfirmationRequest) (project.WorkbenchMaterialConfirmation, error)
	UpdateWorkbenchAssetPointer(context.Context, contract.ActorContext, contract.ProjectID, project.UpdateWorkbenchAssetPointerRequest) (project.WorkbenchAssetVersionPointer, error)
	GetContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error)
	ListProjects(context.Context, contract.ActorContext) ([]project.Project, error)
	CreateBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, project.CreateBusinessTaskRequest) (project.BusinessTask, error)
	ListBusinessTasks(context.Context, contract.ActorContext, contract.ProjectID) ([]project.BusinessTask, error)
	GetBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, string) (project.BusinessTask, error)
	UpdateBusinessTask(context.Context, contract.ActorContext, contract.ProjectID, string, project.UpdateBusinessTaskRequest) (project.BusinessTask, error)
	CreateOperationalRecord(context.Context, contract.ActorContext, contract.ProjectID, project.UpsertOperationalRecordRequest) (project.OperationalRecord, error)
	ListOperationalRecords(context.Context, contract.ActorContext, contract.ProjectID) ([]project.OperationalRecord, error)
	GetOperationalRecord(context.Context, contract.ActorContext, contract.ProjectID, string) (project.OperationalRecord, error)
	UpsertOperationalRecord(context.Context, contract.ActorContext, contract.ProjectID, string, project.UpsertOperationalRecordRequest) (project.OperationalRecord, error)
	CreateChangeSet(context.Context, contract.ActorContext, contract.ProjectID, project.CreateChangeSetRequest) (project.ChangeSet, error)
	ListChangeSets(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ChangeSet, error)
	GetChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error)
	PreflightChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error)
	ApproveChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, project.ChangeSetApprovalRequest) (project.ChangeSet, error)
	ExecuteChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string) (project.ChangeSet, error)
	RollbackChangeSet(context.Context, contract.ActorContext, contract.ProjectID, string, project.RollbackChangeSetRequest) (project.ChangeSet, error)
	ListAuditEvents(context.Context, contract.ActorContext, contract.ProjectID) ([]project.AuditEvent, error)
}
type ProjectMembershipManager interface {
	ListProjectMembers(context.Context, contract.ActorContext, contract.ProjectID) ([]project.ProjectMembership, error)
	AddProjectMember(context.Context, contract.ActorContext, contract.ProjectID, contract.Principal, string) (project.ProjectMembership, error)
	UpdateProjectMember(context.Context, contract.ActorContext, contract.ProjectID, contract.Principal, project.UpdateProjectMembershipRequest) (project.ProjectMembership, error)
}
type AssetUploadManager interface {
	Create(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, assets.CreateUploadRequest) (assets.CreateUploadResponse, error)
	PutContent(context.Context, contract.ActorContext, contract.ProjectID, string, io.Reader, int64) error
	Finalize(context.Context, contract.RequestContext, contract.ProjectID, string) (assets.UploadSession, error)
	List(context.Context, contract.ActorContext, contract.ProjectID, int) ([]assets.ProjectAsset, error)
	Preview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (assets.SignedRequest, error)
	OpenPreview(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) (io.ReadCloser, assets.ObjectInfo, error)
	Remove(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef) error
	UpsertFeature(context.Context, contract.ActorContext, contract.ProjectID, assets.AssetFeature) (assets.AssetFeature, error)
	GetFeature(context.Context, contract.ActorContext, contract.ProjectID, contract.AssetVersionRef, string) (assets.AssetFeature, error)
	ListFeatures(context.Context, contract.ActorContext, contract.ProjectID, int) ([]assets.AssetFeature, error)
}
type GeneratedIntakeManager interface {
	Create(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, assets.GeneratedAssetIntakeRequest) (assets.GeneratedIntake, error)
	Get(context.Context, contract.ActorContext, contract.ProjectID, string) (assets.GeneratedIntake, error)
}
type RemixPlanManager interface {
	Create(context.Context, contract.ActorContext, contract.ProjectID, remix.CreatePlanRequest) (remix.Plan, error)
	Get(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.Plan, error)
	List(context.Context, contract.ActorContext, contract.ProjectID, int) ([]remix.Plan, error)
	CreateRenderJob(context.Context, contract.ActorContext, contract.ProjectID, contract.IdempotencyKey, remix.CreateRenderJobRequest) (remix.RenderJob, error)
	GetRenderJob(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.RenderJob, error)
	CreateQualityReport(context.Context, contract.ActorContext, contract.ProjectID, remix.CreateQualityReportRequest) (remix.QualityReport, error)
	GetQualityReport(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.QualityReport, error)
	GetQualityReportForRenderJob(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.QualityReport, error)
	CreateHitAnalysis(context.Context, contract.ActorContext, contract.ProjectID, remix.CreateHitAnalysisRequest) (remix.HitAnalysis, error)
	GetHitAnalysis(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.HitAnalysis, error)
	CreateProductMapping(context.Context, contract.ActorContext, contract.ProjectID, remix.CreateProductMappingRequest) (remix.ProductMapping, error)
	GetProductMapping(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.ProductMapping, error)
	GeneratePlanFromProductMapping(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.Plan, error)
	CreatePreroll(context.Context, contract.ActorContext, contract.ProjectID, remix.CreatePrerollRequest) (remix.Preroll, error)
	GetPreroll(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.Preroll, error)
	ApplyPreroll(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.Plan, error)
	CreateFeedbackEvent(context.Context, contract.ActorContext, contract.ProjectID, remix.CreateFeedbackEventRequest) (remix.FeedbackEvent, error)
	ListFeedbackEvents(context.Context, contract.ActorContext, contract.ProjectID, remix.FeedbackEventFilter) ([]remix.FeedbackEvent, error)
	GetAssetPerformanceSnapshot(context.Context, contract.ActorContext, contract.ProjectID) ([]remix.AssetPerformance, error)
	CreatePlannerWeightSnapshot(context.Context, contract.ActorContext, contract.ProjectID) (remix.PlannerWeightSnapshot, error)
}
type EvalManager interface {
	CreateEvalCase(context.Context, contract.ActorContext, contract.ProjectID, remix.CreateEvalCaseRequest) (remix.EvalCase, error)
	ListEvalCases(context.Context, contract.ActorContext, contract.ProjectID) ([]remix.EvalCase, error)
	CreateEvalRun(context.Context, contract.ActorContext, contract.ProjectID, remix.CreateEvalRunRequest) (remix.EvalRun, error)
	GetEvalRun(context.Context, contract.ActorContext, contract.ProjectID, string) (remix.EvalRun, error)
}
type AgentRunManager interface {
	CreateRun(context.Context, contract.ActorContext, contract.ProjectID, agent.CreateRunRequest) (agent.AgentRun, error)
	ListRuns(context.Context, contract.ActorContext, contract.ProjectID, int) ([]agent.AgentRun, error)
	GetRun(context.Context, contract.ActorContext, contract.ProjectID, string) (agent.AgentRun, error)
	CancelRun(context.Context, contract.ActorContext, contract.ProjectID, string) (agent.AgentRun, error)
}
type KnowledgeManager interface {
	ImportDocument(context.Context, contract.ActorContext, contract.ProjectID, knowledge.ImportDocumentRequest) (knowledge.Document, error)
	ListDocuments(context.Context, contract.ActorContext, contract.ProjectID, int) ([]knowledge.Document, error)
	GetDocument(context.Context, contract.ActorContext, contract.ProjectID, string) (knowledge.Document, error)
	ExtractDocumentMedia(context.Context, contract.ActorContext, contract.ProjectID, string) ([]knowledge.ExtractedDocumentMedia, error)
	Search(context.Context, contract.ActorContext, contract.ProjectID, knowledge.SearchRequest) ([]knowledge.SearchResult, error)
	CreateDocument(context.Context, contract.ActorContext, contract.ProjectID, string, string, io.Reader, int64) (knowledge.Document, error)
	RunResearch(context.Context, contract.ActorContext, contract.ProjectID, knowledge.ResearchRequest) (knowledge.ResearchRun, error)
	GetResearchRun(context.Context, contract.ActorContext, contract.ProjectID, string) (knowledge.ResearchRun, error)
	ListResearchRuns(context.Context, contract.ActorContext, contract.ProjectID, int) ([]knowledge.ResearchRun, error)
	ListResearchArtifacts(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]knowledge.ResearchArtifact, error)
	GetResearchArtifact(context.Context, contract.ActorContext, contract.ProjectID, string) (knowledge.ResearchArtifact, error)
}

// CreativeManager is the public application seam from the shared HTTP host to
// the Creative bounded context. It keeps the host unaware of Creative SQL.
type CreativeManager interface {
	ListCommercePrerollSources(context.Context, contract.ActorContext, contract.ProjectID) ([]creative.CreativeSourceOption, error)
	PrepareCommercePreroll(context.Context, contract.ActorContext, contract.ProjectID, creative.PrepareCommercePrerollRequest) (creative.PreparedCommercePreroll, error)
	EnsureCommerceFixtureWorkspace(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, creative.EnsureCommerceFixtureWorkspaceRequest) (creative.TaskDetail, error)
	GetLatestCommerceWorkspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error)
	GetCommerceWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	UpdateCommercePrerollDraft(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateCommercePrerollDraftRequest) (creative.TaskDetail, error)
	ConfirmCommerceGeneration(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmCommerceGenerationRequest) (creative.TaskDetail, error)
	CommerceProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error)
	RegisterCommerceGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.CommerceGenerationAttempt, error)
	EnsureBrandFilmFixtureWorkspace(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey) (creative.TaskDetail, error)
	GetLatestBrandFilmWorkspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error)
	GetBrandFilmWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	InitializeStrategyBrandFilmWorkspace(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	AnalyzeBrandFilmBrief(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	UpdateBrandFilmBrief(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandBriefAnalysisRequest) (creative.TaskDetail, error)
	ConfirmBrandFilmBrief(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	GenerateBrandFilmConcepts(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	UpdateBrandFilmConcepts(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandConceptsRequest) (creative.TaskDetail, error)
	SelectBrandFilmConcept(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectBrandConceptRequest) (creative.TaskDetail, error)
	GenerateBrandFilmPlan(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	UpdateBrandFilmPlan(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandFilmPlanRequest) (creative.TaskDetail, error)
	ConfirmBrandFilmPlan(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	PrepareBrandFilmGeneration(context.Context, contract.ActorContext, contract.ProjectID, string, creative.PrepareBrandFilmGenerationRequest) (creative.TaskDetail, error)
	RegenerateBrandFilmUnit(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RegenerateBrandFilmUnitRequest) (creative.TaskDetail, error)
	BrandFilmProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string, string) (provider.VideoGenerationInput, string, error)
	RegisterBrandFilmGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string, string) (creative.TaskDetail, error)
	ReconcileBrandFilmGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string, contract.ProviderJob) (creative.TaskDetail, error)
	LockBrandFilmGenerationUnit(context.Context, contract.ActorContext, contract.ProjectID, string, creative.LockBrandFilmUnitRequest) (creative.TaskDetail, error)
	ComposeBrandFilmPreview(context.Context, contract.RequestContext, contract.ProjectID, string, creative.ComposeBrandFilmPreviewRequest) (creative.TaskDetail, error)
	PrepareBrandFilmAudio(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	MaterializeBrandFilmAudioAssets(context.Context, contract.RequestContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	UpdateBrandFilmAudioMix(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateBrandAudioMixRequest) (creative.TaskDetail, error)
	SelectBrandFilmAudioVariant(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectBrandAudioVariantRequest) (creative.TaskDetail, error)
	RenderBrandFilmAudioPreview(context.Context, contract.RequestContext, contract.ProjectID, string, creative.BrandFilmRevisionRequest) (creative.TaskDetail, error)
	GenerateBrandFilmVoiceClip(context.Context, contract.RequestContext, contract.ProjectID, string, creative.GenerateBrandVoiceClipRequest) (creative.TaskDetail, error)
	ProbeBrandFilmSpeech(context.Context, contract.ActorContext, contract.ProjectID) (provider.SpeechCapability, error)
	RunBrandFilmQuality(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RunBrandFilmQualityRequest) (creative.TaskDetail, error)
	ConfirmBrandFilmQuality(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmBrandFilmQualityRequest) (creative.TaskDetail, error)
	FinalizeBrandFilmVersion(context.Context, contract.RequestContext, contract.ProjectID, string, creative.BrandFilmVersionRequest, contract.IdempotencyKey) (creative.BrandFilmVersionResult, error)
	ApproveBrandFilmVersion(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmVersionRequest) (creative.BrandFilmVersionResult, error)
	DeliverBrandFilmVersion(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BrandFilmVersionRequest) (creative.BrandFilmDeliveryResult, error)
	CreateIntake(context.Context, contract.RequestContext, contract.ProjectID, contract.IdempotencyKey, creative.CreateIntakeRequest) (creative.CreativeIntake, error)
	ListIntakes(context.Context, contract.ActorContext, contract.ProjectID, int) ([]creative.CreativeIntake, error)
	GetIntake(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativeIntake, error)
	ListBusinessCapabilities(context.Context, contract.ActorContext, contract.ProjectID) ([]creative.CreativeBusinessCapability, error)
	CreateTask(context.Context, contract.ActorContext, contract.ProjectID, string, creative.CreateTaskRequest) (creative.CreativeTask, error)
	CreateVideoTask(context.Context, contract.ActorContext, contract.ProjectID, string, creative.CreateVideoTaskRequest) (creative.CreativeTask, error)
	ListTasks(context.Context, contract.ActorContext, contract.ProjectID, int) ([]creative.CreativeTask, error)
	RenameTask(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RenameTaskRequest) (creative.CreativeTask, error)
	GetTaskDetail(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	GetLatestShortDramaWorkspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error)
	SelectShortDramaCandidate(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectShortDramaCandidateRequest) (creative.TaskDetail, error)
	RegenerateShortDramaCandidates(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RegenerateShortDramaCandidatesRequest) (creative.TaskDetail, error)
	ShortDramaProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error)
	RegisterShortDramaGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.ShortDramaGenerationAttempt, error)
	GetLatestGamePrerollWorkspace(context.Context, contract.ActorContext, contract.ProjectID) (creative.TaskDetail, error)
	PrepareGamePrerollEvidence(context.Context, contract.RequestContext, contract.ProjectID, string, creative.PrepareGamePrerollEvidenceRequest) (creative.TaskDetail, error)
	SelectGamePrerollCandidate(context.Context, contract.ActorContext, contract.ProjectID, string, creative.SelectGamePrerollCandidateRequest) (creative.TaskDetail, error)
	RegenerateGamePrerollCandidates(context.Context, contract.ActorContext, contract.ProjectID, string, creative.RegenerateGamePrerollCandidatesRequest) (creative.TaskDetail, error)
	GamePrerollProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error)
	RegisterGamePrerollGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.GamePrerollGenerationAttempt, error)
	AnalyzeViralRemake(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	UpdateViralPrompt(context.Context, contract.ActorContext, contract.ProjectID, string, creative.UpdateViralPromptRequest) (creative.TaskDetail, error)
	ConfirmViralGeneration(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ConfirmViralGenerationRequest) (creative.TaskDetail, error)
	ViralProviderInput(context.Context, contract.ActorContext, contract.ProjectID, string) (provider.VideoGenerationInput, string, error)
	RegisterViralCandidateJob(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.TaskDetail, error)
	ReconcileViralCandidate(context.Context, contract.ActorContext, contract.ProjectID, string, contract.ProviderJob) (creative.TaskDetail, error)
	SubmitViralCandidateReview(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.TaskDetail, error)
	ArchiveTask(context.Context, contract.ActorContext, contract.ProjectID, string) error
	ReviseDraft(context.Context, contract.ActorContext, contract.ProjectID, string, creative.ReviseDraftRequest) (creative.ImageTextDraft, error)
	BindImageAsset(context.Context, contract.ActorContext, contract.ProjectID, string, creative.BindImageAssetRequest) (creative.ImageTextDraft, error)
	RegisterCoverImageJob(context.Context, contract.ActorContext, contract.ProjectID, string, string) error
	RegisterImagePlanJob(context.Context, contract.ActorContext, contract.ProjectID, string, int, string) error
	RegisterVideoJob(context.Context, contract.ActorContext, contract.ProjectID, string, string) error
	CreateRenderJob(context.Context, contract.RequestContext, contract.ProjectID, string, creative.CreateRenderJobRequest, contract.IdempotencyKey) (creative.RenderJob, bool, error)
	GetRenderJob(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.RenderJob, error)
	FreezeVersion(context.Context, contract.RequestContext, contract.ProjectID, string, creative.FreezeVersionRequest, contract.IdempotencyKey) (creative.CreativeVersion, bool, error)
	ListVersions(context.Context, contract.ActorContext, contract.ProjectID, string, int) ([]creative.CreativeVersion, error)
	CheckVersion(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativeVersion, error)
	ApproveVersion(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativeVersion, error)
	DeliverVersion(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.CreativePackage, error)
	ListPackages(context.Context, contract.ActorContext, contract.ProjectID, int) ([]creative.CreativePackage, error)
}

// ProviderJobs keeps the shared HTTP server dependent on Provider's public
// application seam, rather than its SQL store or vendor adapters.
type ProviderJobs interface {
	CreateImageJob(context.Context, provider.CreateImageJobRequest) (contract.ProviderJob, bool, error)
	CreateVideoJob(context.Context, provider.CreateVideoJobRequest) (contract.ProviderJob, bool, error)
	GetJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (contract.ProviderJob, error)
}

type ProviderConfigurationReader interface {
	ListCapabilities(context.Context, contract.OrganizationID) ([]provider.CapabilityStatus, error)
}

// New retains the bootstrap construction path for focused HTTP tests. The
// application uses NewWithDependencies so readiness and project checks are real.
func New(resolver identity.Resolver) *Server {
	return NewWithDependencies(Dependencies{Resolver: resolver})
}

func NewWithDependencies(dependencies Dependencies) *Server {
	if dependencies.Resolver == nil {
		dependencies.Resolver = identity.RejectingResolver{}
	}
	if dependencies.ProjectAuthorizer == nil {
		dependencies.ProjectAuthorizer = identity.RejectingProjectAuthorizer{}
	}
	server := &Server{
		resolver: dependencies.Resolver, projectAuthorizer: dependencies.ProjectAuthorizer,
		providerJobs: dependencies.ProviderJobs, readiness: dependencies.Readiness,
		identities: dependencies.Identities, accounts: dependencies.Accounts, projects: dependencies.Projects,
		projectMembers: dependencies.ProjectMembers, uploads: dependencies.Uploads,
		intakes: dependencies.Intakes, newID: newRequestID,
		creative: dependencies.Creative, productionCenter: dependencies.ProductionCenter, productionAssets: dependencies.ProductionAssets, productionRetry: dependencies.ProductionRetry, sessions: dependencies.Sessions, knowledge: dependencies.Knowledge,
		remixPlans: dependencies.RemixPlans, evals: dependencies.Evals, agentRuns: dependencies.AgentRuns,
		providerConfig: dependencies.ProviderConfig,
	}
	server.mux = http.NewServeMux()
	server.mux.HandleFunc("GET /healthz", server.health)
	server.mux.HandleFunc("GET /readyz", server.ready)
	server.mux.HandleFunc("POST /platform/v1/auth/login", server.login)
	server.mux.HandleFunc("POST /platform/v1/auth/logout", server.logout)
	server.mux.Handle("POST /platform/v1/auth/switch-organization", server.requireAuthentication(http.HandlerFunc(server.switchOrganization)))
	server.mux.Handle("GET /platform/v1/context", server.requireAuthentication(http.HandlerFunc(server.requestContext)))
	server.mux.Handle("GET /platform/v1/me", server.requireAuthentication(http.HandlerFunc(server.currentIdentity)))
	server.mux.Handle("PATCH /platform/v1/me", server.requireAuthentication(server.requireScope("identity.profile.write", http.HandlerFunc(server.updateCurrentIdentity))))
	server.mux.Handle("GET /platform/v1/organizations", server.requireAuthentication(server.requireScope("organization.read", http.HandlerFunc(server.listOrganizations))))
	server.mux.Handle("GET /platform/v1/organizations/{organization_id}/members", server.requireAuthentication(server.requireScope("organization.members.read", http.HandlerFunc(server.listOrganizationMembers))))
	server.mux.Handle("POST /platform/v1/organizations/{organization_id}/members", server.requireAuthentication(server.requireScope("organization.members.manage", http.HandlerFunc(server.addOrganizationMember))))
	server.mux.Handle("PATCH /platform/v1/organizations/{organization_id}/members/{user_id}", server.requireAuthentication(server.requireScope("organization.members.manage", http.HandlerFunc(server.updateOrganizationMember))))
	server.mux.Handle("GET /platform/v1/provider/capabilities", server.requireAuthentication(http.HandlerFunc(server.providerCapabilities)))
	server.mux.Handle("POST /platform/v1/brands", server.requireAuthentication(server.requireScope("project.write", http.HandlerFunc(server.createBrand))))
	server.mux.Handle("POST /platform/v1/projects", server.requireAuthentication(server.requireScope("project.write", http.HandlerFunc(server.createProject))))
	server.mux.Handle("GET /platform/v1/projects", server.requireAuthentication(server.requireScope("project.read", http.HandlerFunc(server.listProjects))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.projectDetail))))
	server.mux.Handle("PATCH /platform/v1/projects/{project_id}", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.updateProject))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/artifacts", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.listProjectArtifacts))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/artifacts", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.createProjectArtifact))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/artifacts/{artifact_id}", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.getProjectArtifact))))
	server.mux.Handle("PATCH /platform/v1/projects/{project_id}/artifacts/{artifact_id}", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.updateProjectArtifact))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/workbench", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.projectWorkbench))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/members", server.requireProject(server.requireScope("project.members.read", http.HandlerFunc(server.listProjectMembers))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/members", server.requireProject(server.requireScope("project.members.manage", http.HandlerFunc(server.addProjectMember))))
	server.mux.Handle("PATCH /platform/v1/projects/{project_id}/members/{principal_kind}/{principal_id}", server.requireProject(server.requireScope("project.members.manage", http.HandlerFunc(server.updateProjectMember))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}/quality-checks", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.runWorkbenchQualityCheck))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}/confirmations", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.recordWorkbenchMaterialConfirmation))))
	server.mux.Handle("PATCH /platform/v1/projects/{project_id}/assets/{asset_id}/version-pointer", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.updateWorkbenchAssetPointer))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/context", server.requireProject(http.HandlerFunc(server.projectContext)))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/tasks", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.listProjectTasks))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/tasks", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.createProjectTask))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/tasks/{task_id}", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.getProjectTask))))
	server.mux.Handle("PATCH /platform/v1/projects/{project_id}/tasks/{task_id}", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.updateProjectTask))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/operations", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.listProjectOperations))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/operations", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.createProjectOperation))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/operations/{operation_id}", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.getProjectOperation))))
	server.mux.Handle("PUT /platform/v1/projects/{project_id}/operations/{operation_id}", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.upsertProjectOperation))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/change-sets", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.listProjectChangeSets))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/change-sets", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.createProjectChangeSet))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/change-sets/{change_set_id}", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.getProjectChangeSet))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/change-sets/{change_set_id}/preflight", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.preflightProjectChangeSet))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/change-sets/{change_set_id}/approve", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.approveProjectChangeSet))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/change-sets/{change_set_id}/execute", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.executeProjectChangeSet))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/change-sets/{change_set_id}/rollback", server.requireProject(server.requireScope("project.write", http.HandlerFunc(server.rollbackProjectChangeSet))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/audit-events", server.requireProject(server.requireScope("project.read", http.HandlerFunc(server.listProjectAuditEvents))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/assets/uploads", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.createUpload))))
	server.mux.Handle("PUT /platform/v1/projects/{project_id}/assets/uploads/{upload_id}", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.putUpload))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/assets/uploads/{upload_action}", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.finalizeUpload))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/assets", server.requireProject(server.requireScope("assets.read", http.HandlerFunc(server.listAssets))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/assets/features", server.requireProject(server.requireScope("assets.read", http.HandlerFunc(server.listAssetFeatures))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}/preview", server.requireProject(server.requireScope("assets.read", http.HandlerFunc(server.previewAsset))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}/content", server.requireProject(server.requireScope("assets.read", http.HandlerFunc(server.assetContent))))
	server.mux.Handle("DELETE /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.removeAsset))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}/features/{feature_version}", server.requireProject(server.requireScope("assets.read", http.HandlerFunc(server.getAssetFeature))))
	server.mux.Handle("PUT /platform/v1/projects/{project_id}/assets/{asset_id}/versions/{version}/features/{feature_version}", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.putAssetFeature))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/assets/generated-intakes", server.requireProject(server.requireScope("assets.write", http.HandlerFunc(server.createGeneratedIntake))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/assets/generated-intakes/{intake_id}", server.requireProject(server.requireScope("assets.read", http.HandlerFunc(server.getGeneratedIntake))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/knowledge/documents", server.requireProject(server.requireScope(knowledge.ScopeWrite, http.HandlerFunc(server.knowledgeDocumentEntry))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/documents", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.listKnowledgeDocuments))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/documents/{document_id}", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.getKnowledgeDocument))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/knowledge/documents/{document_id}/media:extract", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.extractKnowledgeDocumentMedia))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/search", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.searchKnowledge))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/knowledge/research-runs", server.requireProject(server.requireScope("strategy.write", http.HandlerFunc(server.runKnowledgeResearch))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/research-runs", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.listKnowledgeResearchRuns))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/research-runs/{research_run_id}", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.getKnowledgeResearchRun))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/research-artifacts", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.listKnowledgeResearchArtifacts))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/knowledge/research-artifacts/{artifact_id}", server.requireProject(server.requireScope(knowledge.ScopeRead, http.HandlerFunc(server.getKnowledgeResearchArtifact))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-plans", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixPlan))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-plans", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.listRemixPlans))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-plans/{plan_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixPlan))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-render-jobs", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixRenderJob))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-render-jobs/{job_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixRenderJob))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-render-jobs/{job_id}/quality-report", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixRenderJobQualityReport))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-quality-reports", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixQualityReport))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-quality-reports/{report_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixQualityReport))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-hit-analyses", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixHitAnalysis))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-hit-analyses/{analysis_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixHitAnalysis))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-product-mappings", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixProductMapping))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-product-mappings/{mapping_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixProductMapping))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-product-mappings/{mapping_id}/plans", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.generateRemixPlanFromProductMapping))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-prerolls", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixPreroll))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-prerolls/{preroll_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixPreroll))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-prerolls/{preroll_id}/apply", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.applyRemixPreroll))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-feedback-events", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixFeedbackEvent))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-feedback-events", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.listRemixFeedbackEvents))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-asset-performance", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixAssetPerformance))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-planner-weight-snapshots", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixPlannerWeightSnapshot))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-eval-cases", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.listRemixEvalCases))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-eval-cases", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixEvalCase))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/remix-eval-runs", server.requireProject(server.requireScope(remix.ScopePlanWrite, http.HandlerFunc(server.createRemixEvalRun))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/remix-eval-runs/{run_id}", server.requireProject(server.requireScope(remix.ScopePlanRead, http.HandlerFunc(server.getRemixEvalRun))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/agent-runs", server.requireProject(server.requireScope(agent.ScopeRunRead, http.HandlerFunc(server.listAgentRuns))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/agent-runs", server.requireProject(server.requireScope(agent.ScopeRunWrite, http.HandlerFunc(server.createAgentRun))))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/agent-runs/{agent_run_id}", server.requireProject(server.requireScope(agent.ScopeRunRead, http.HandlerFunc(server.getAgentRun))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/agent-runs/{agent_run_id}/cancel", server.requireProject(server.requireScope(agent.ScopeRunWrite, http.HandlerFunc(server.cancelAgentRun))))
	server.mux.Handle("POST /platform/v1/projects/{project_id}/model/jobs", server.requireProject(http.HandlerFunc(server.createImageJob)))
	server.mux.Handle("GET /platform/v1/projects/{project_id}/model/jobs/{job_id}", server.requireProject(http.HandlerFunc(server.getProviderJob)))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-intakes", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listCreativeIntakes))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/production-runs", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listProductionRuns))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/production-runs/{production_source}/{production_run_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getProductionRun))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/production-runs/{production_source}/{production_run_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.retryProductionRun))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/production-assets", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listProductionAssets))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-intakes", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.createCreativeIntake))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getCreativeIntake))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}/brand-brief:prepare", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.prepareCreativeBrandBrief))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}/brand-brief", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getCreativeBrandBrief))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}/brand-brief", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateCreativeBrandBrief))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}/brand-brief:confirm", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmCreativeBrandBrief))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}/direction-candidate-batches/latest", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getLatestCreativeDirectionBatch))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-intakes/{intake_id}/direction-candidate-batches", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.createCreativeDirectionBatch))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-directions/{direction_id}/confirm", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmCreativeDirection))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/business-capabilities", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listCreativeBusinessCapabilities))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/commerce-preroll/sources", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listCommercePrerollSources))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/commerce-preroll:prepare", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.prepareCommercePreroll))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-workspaces/commerce-preroll:ensure-fixture", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.ensureCommerceFixtureWorkspace))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-workspaces/brand-film:ensure-fixture", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.ensureBrandFilmFixtureWorkspace))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/requirements:analyze", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.analyzeAINativeRequirement))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/products:resolve", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.resolveAINativeProductPreview))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/ai-native-ads", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listAINativeAdWorkspaces))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/ai-native-ads:latest", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getLatestAINativeRequirementWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getAINativeRequirementWorkspace))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/metadata", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.renameAINativeAdWorkspace))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/requirement", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateAINativeRequirement))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/requirement:confirm", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmAINativeRequirement))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/reopen-impact", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getAINativeReopenImpact))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/requirement:reopen", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.reopenAINativeRequirement))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/script:generate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateAINativeScript))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/script", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateAINativeScript))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/script:regenerate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateAINativeScript))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/script:confirm", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmAINativeScript))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/script:reopen", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.reopenAINativeScript))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/storyboard:generate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateAINativeStoryboard))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/storyboard/assets/{asset_id}/regenerate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.regenerateAINativeStoryboardAsset))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/storyboard", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateAINativeStoryboard))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/storyboard:confirm", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmAINativeStoryboard))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/storyboard:reopen", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.reopenAINativeStoryboard))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/storyboard/voiceover:fit", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.suggestAINativeVoiceoverFit))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/production:start", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.startAINativeProduction))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/production/units/{unit_id}/retry", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.retryAINativeProductionUnit))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/ai-native-ads/{workspace_id}/production:cancel", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.cancelAINativeProduction))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-intakes/{intake_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.createCreativeTask))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listCreativeTasks))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/edit-tasks", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.createEditTask))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/edit-tasks", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listEditTasks))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/edit-tasks/{edit_task_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getEditTask))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/edit-tasks/{edit_task_id}/timeline", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.saveEditTimeline))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/edit-tasks/{edit_task_id}/operations:batch", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.applyEditOperations))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/edit-tasks/{edit_task_id}/timeline-versions", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listEditTimelineVersions))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/edit-tasks/{edit_task_id}/renders", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.createEditingRender))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/edit-tasks/{edit_task_id}/versions:submit", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.submitEditingVersion))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/edit-renders/{render_job_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getEditingRender))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/edit-renders/{render_job_id}/cancel", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.cancelEditingRender))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/edit-renders/{render_job_id}/retry", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.retryEditingRender))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/image-text-workspace", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getCreativeImageTextWorkspace))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/image-text-draft:generate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateCreativeImageTextDraft))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/image-text-draft", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateCreativeImageTextDraft))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/image-slots/{slot_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateCreativeImageTextSlot))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/image-slots/{order}/attempts/{attempt_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.adoptCreativeImageTextSlot))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-workspaces/commerce-preroll", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getLatestCommercePrerollWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-workspaces/short-drama-preroll", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getLatestShortDramaPrerollWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-workspaces/game-preroll", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getLatestGamePrerollWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-workspaces/brand-film", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getLatestBrandFilmWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getBrandFilmWorkspace))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:initialize-from-strategy", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.initializeStrategyBrandFilmWorkspace))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:analyze-brief", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.analyzeBrandFilmBrief))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film/brief", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateBrandFilmBrief))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:confirm-brief", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmBrandFilmBrief))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:generate-concepts", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateBrandFilmConcepts))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film/concepts", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateBrandFilmConcepts))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:select-concept", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.selectBrandFilmConcept))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:generate-plan", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateBrandFilmPlan))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film/plan", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateBrandFilmPlan))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:confirm-plan", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmBrandFilmPlan))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:prepare-generation", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.prepareBrandFilmGeneration))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:generate-unit", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateBrandFilmUnit))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:reconcile-unit", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.reconcileBrandFilmUnit))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:lock-unit", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.lockBrandFilmUnit))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:compose-preview", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.composeBrandFilmPreview))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film/audio", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getBrandFilmAudio))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:prepare-audio", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.prepareBrandFilmAudio))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:materialize-audio-assets", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.materializeBrandFilmAudioAssets))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film/audio/mix", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateBrandFilmAudioMix))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film/audio:select-variant", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.selectBrandFilmAudioVariant))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:render-audio-preview", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.renderBrandFilmAudioPreview))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:generate-voice", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.generateBrandFilmVoiceClip))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/brand-film/speech-capability", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.probeBrandFilmSpeech))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:run-quality", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.runBrandFilmQuality))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:confirm-quality", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmBrandFilmQuality))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:finalize-version", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.finalizeBrandFilmVersion))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:approve-version", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.approveBrandFilmVersion))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/brand-film:deliver-version", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.deliverBrandFilmVersion))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/viral-remake", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getViralRemakeWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getShortDramaPrerollWorkspace))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getCommercePrerollWorkspace))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-draft", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateCommercePrerollDraft))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll:confirm-generation", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmCommerceGeneration))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/commerce-preroll-v2", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/commerce-preroll-v2:latest", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:analyze-source", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:confirm-understanding", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:generate-hooks", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:select-hook", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:prepare-references", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:bind-product-reference", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:select-product-reference", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:update-storyboard", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:update-prompt", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2/versions", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:save-version", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:restore-version", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:bind-custom-first-frame", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:generate-first-frames", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:reconcile-first-frame", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:select-first-frame", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:generate-video", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:reconcile-video", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/commerce-preroll-v2:adopt-output", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.commercePrerollV2))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll:select-candidate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.selectShortDramaCandidate))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll:regenerate-candidates", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.regenerateShortDramaCandidates))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:analyze-source", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:update-analysis", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:generate-directions", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:select-direction", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:update-prompts", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:prepare-opening-frame", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:generate-first-frames", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:reconcile-first-frame", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:select-first-frame", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:bind-trusted-materials", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:reconcile-video", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:generate-video", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.shortDramaV2Command))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/short-drama-preroll-v2:open-editor", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.openShortDramaV2Editor))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/open-editor", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.openCreativeVersionEditor))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/game-preroll:select-candidate", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.selectGamePrerollCandidate))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/game-preroll:prepare-evidence", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.prepareGamePrerollEvidence))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/game-preroll:regenerate-candidates", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.regenerateGamePrerollCandidates))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/viral-remake:analyze-reference", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.analyzeViralRemake))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/viral-remake/prompt-draft", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.updateViralPrompt))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/viral-remake:confirm-generation", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.confirmViralGeneration))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/viral-remake/candidates/{candidate_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.transitionViralCandidate))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getCreativeTask))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}/metadata", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.renameCreativeTask))))
	server.mux.Handle("DELETE /api/creative/v1/projects/{project_id}/creative-tasks/{task_id}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.archiveCreativeTask))))
	server.mux.Handle("PATCH /api/creative/v1/projects/{project_id}/creative-tasks/{task_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.reviseCreativeDraft))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-tasks/{task_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.createCreativeCoverImageJob))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-render-jobs/{render_job_id}", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.getCreativeRenderJob))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-versions", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listCreativeVersions))))
	server.mux.Handle("POST /api/creative/v1/projects/{project_id}/creative-versions/{version_action}", server.requireProject(server.requireScope(creative.ScopeWrite, http.HandlerFunc(server.transitionCreativeVersion))))
	server.mux.Handle("GET /api/creative/v1/projects/{project_id}/creative-packages", server.requireProject(server.requireScope(creative.ScopeRead, http.HandlerFunc(server.listCreativePackages))))
	for _, mount := range dependencies.AuthenticatedDomainMounts {
		if strings.TrimSpace(mount.Pattern) == "" || mount.Handler == nil {
			continue
		}
		server.mux.Handle(mount.Pattern, server.requireAuthentication(mount.Handler))
	}
	server.mux.HandleFunc("/", server.notFound)
	return server
}

func (s *Server) login(writer http.ResponseWriter, request *http.Request) {
	if s.sessions == nil {
		s.notImplemented(writer, request)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 8<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || strings.TrimSpace(body.Username) == "" ||
		body.Password == "" {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_REQUEST", Message: "请输入账号和密码",
			RequestID: requestIDFrom(request.Context()), Retryable: false,
		})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_REQUEST", Message: "登录请求格式无效",
			RequestID: requestIDFrom(request.Context()), Retryable: false,
		})
		return
	}
	result, err := s.sessions.Login(request.Context(), body.Username, body.Password)
	if err != nil {
		status := http.StatusUnauthorized
		code := "INVALID_CREDENTIALS"
		if errors.Is(err, identity.ErrCredentialLocked) {
			status, code = http.StatusTooManyRequests, "LOGIN_RATE_LIMITED"
		}
		if !errors.Is(err, identity.ErrInvalidCredentials) && !errors.Is(err, identity.ErrCredentialLocked) {
			status, code = http.StatusServiceUnavailable, "IDENTITY_UNAVAILABLE"
		}
		writeProblem(writer, status, contract.Error{
			Code: code, Message: "账号或密码错误，请稍后重试",
			RequestID: requestIDFrom(request.Context()), Retryable: status >= 500,
		})
		return
	}
	http.SetCookie(writer, s.sessions.Cookie(result.Token, result.ExpiresAt))
	writeJSON(writer, http.StatusOK, map[string]any{
		"actor": result.Actor, "expires_at": result.ExpiresAt,
	})
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if s.sessions == nil {
		s.notImplemented(writer, request)
		return
	}
	if cookie, err := request.Cookie(identity.SessionCookieName); err == nil {
		if len(cookie.Value) > 128 {
			http.SetCookie(writer, s.sessions.ExpiredCookie())
			writeProblem(writer, http.StatusBadRequest, contract.Error{
				Code: "INVALID_SESSION_COOKIE", Message: "会话 Cookie 格式无效",
				RequestID: requestIDFrom(request.Context()), Retryable: false,
			})
			return
		}
		if err := s.sessions.Logout(request.Context(), cookie.Value); err != nil {
			writeProblem(writer, http.StatusServiceUnavailable, contract.Error{
				Code: "IDENTITY_UNAVAILABLE", Message: "暂时无法退出，请稍后重试",
				RequestID: requestIDFrom(request.Context()), Retryable: true,
			})
			return
		}
	}
	http.SetCookie(writer, s.sessions.ExpiredCookie())
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) switchOrganization(writer http.ResponseWriter, request *http.Request) {
	if s.sessions == nil {
		s.notImplemented(writer, request)
		return
	}
	var body struct {
		OrganizationID contract.OrganizationID `json:"organization_id"`
	}
	if err := decodeJSON(writer, request, &body); err != nil || strings.TrimSpace(string(body.OrganizationID)) == "" {
		s.badRequest(writer, request, err)
		return
	}
	cookie, err := request.Cookie(identity.SessionCookieName)
	if err != nil || len(cookie.Value) > 128 {
		writeProblem(writer, http.StatusUnauthorized, contract.Error{
			Code: "UNAUTHENTICATED", Message: "当前会话无效",
			RequestID: requestIDFrom(request.Context()), Retryable: false,
		})
		return
	}
	result, err := s.sessions.SwitchOrganization(request.Context(), cookie.Value, body.OrganizationID)
	if err != nil {
		status, code := http.StatusForbidden, "ORGANIZATION_ACCESS_DENIED"
		if errors.Is(err, identity.ErrUnauthenticated) {
			status, code = http.StatusUnauthorized, "UNAUTHENTICATED"
		} else if !errors.Is(err, identity.ErrActorInactive) {
			status, code = http.StatusServiceUnavailable, "IDENTITY_UNAVAILABLE"
		}
		writeProblem(writer, status, contract.Error{
			Code: code, Message: "无法切换到指定组织",
			RequestID: requestIDFrom(request.Context()), Retryable: status >= 500,
		})
		return
	}
	http.SetCookie(writer, s.sessions.Cookie(result.Token, result.ExpiresAt))
	writeJSON(writer, http.StatusOK, map[string]any{"actor": result.Actor, "expires_at": result.ExpiresAt})
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	requestID := validOpaqueID(request.Header.Get("X-Request-ID"))
	if requestID == "" {
		var err error
		requestID, err = s.newID()
		if err != nil {
			writeProblem(writer, http.StatusInternalServerError, contract.Error{
				Code:      "INTERNAL",
				Message:   "服务暂时不可用，请稍后重试",
				Retryable: true,
			})
			return
		}
	}
	writer.Header().Set("X-Request-ID", requestID)
	request = request.WithContext(withRequestID(request.Context(), requestID))
	s.mux.ServeHTTP(writer, request)
}

func (s *Server) health(writer http.ResponseWriter, request *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) ready(writer http.ResponseWriter, request *http.Request) {
	if s.readiness != nil {
		checkContext, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := s.readiness.Check(checkContext); err != nil {
			writeProblem(writer, http.StatusServiceUnavailable, contract.Error{
				Code:      "DEPENDENCY_UNAVAILABLE",
				Message:   "服务依赖暂时不可用，请稍后重试",
				RequestID: requestIDFrom(request.Context()),
				Retryable: true,
			})
			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) requireAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		actor, err := s.resolver.Authenticate(request.Context(), request)
		if err != nil {
			if errors.Is(err, identity.ErrUnauthenticated) {
				writeProblem(writer, http.StatusUnauthorized, contract.Error{
					Code:      "UNAUTHENTICATED",
					Message:   "需要有效身份后才能访问该资源",
					RequestID: requestIDFrom(request.Context()),
					Retryable: false,
				})
				return
			}
			writeProblem(writer, http.StatusInternalServerError, contract.Error{
				Code:      "IDENTITY_UNAVAILABLE",
				Message:   "身份服务暂时不可用，请稍后重试",
				RequestID: requestIDFrom(request.Context()),
				Retryable: true,
			})
			return
		}

		requestContext := contract.RequestContext{
			RequestID: requestIDFrom(request.Context()),
			TraceID:   traceID(request.Header.Get("traceparent"), requestIDFrom(request.Context())),
			Actor:     actor,
		}
		if err := requestContext.Validate(); err != nil {
			writeProblem(writer, http.StatusInternalServerError, contract.Error{
				Code:      "IDENTITY_CONTEXT_INVALID",
				Message:   "身份上下文无效",
				RequestID: requestIDFrom(request.Context()),
				Retryable: false,
			})
			return
		}
		next.ServeHTTP(writer, request.WithContext(contract.WithRequestContext(request.Context(), requestContext)))
	})
}

func (s *Server) requireProject(next http.Handler) http.Handler {
	return s.requireAuthentication(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestContext, ok := contract.RequestContextFrom(request.Context())
		projectID := contract.ProjectID(request.PathValue("project_id"))
		action := projectAction(request)
		if !ok || projectID == "" || s.projectAuthorizer.AuthorizeProjectAction(request.Context(), requestContext.Actor, projectID, action) != nil {
			writeProblem(writer, http.StatusForbidden, contract.Error{Code: "PROJECT_ACCESS_DENIED", Message: "当前身份无权访问该项目", RequestID: requestContext.RequestID, Retryable: false})
			return
		}
		next.ServeHTTP(writer, request)
	}))
}

func projectAction(request *http.Request) string {
	path := request.URL.Path
	if strings.Contains(path, "/members") && request.Method != http.MethodGet && request.Method != http.MethodHead {
		return "manage"
	}
	if strings.Contains(path, "/approve") || strings.Contains(path, "/execute") ||
		strings.Contains(path, "/rollback") || strings.Contains(path, "/confirmations") {
		return "approve"
	}
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		return "read"
	}
	return "write"
}

func (s *Server) requireScope(scope contract.Scope, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestContext, ok := contract.RequestContextFrom(request.Context())
		if !ok || !requestContext.Actor.HasScope(scope) {
			writeProblem(writer, http.StatusForbidden, contract.Error{Code: contract.ErrorScopeRequired, Message: "The required permission scope is missing.", RequestID: requestIDFrom(request.Context()), Retryable: false})
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) requestContext(writer http.ResponseWriter, request *http.Request) {
	requestContext, _ := contract.RequestContextFrom(request.Context())
	writeJSON(writer, http.StatusOK, requestContext)
}

func (s *Server) projectContext(writer http.ResponseWriter, request *http.Request) {
	requestContext, _ := contract.RequestContextFrom(request.Context())
	if s.projects == nil {
		writeJSON(writer, http.StatusOK, map[string]any{"request_context": requestContext, "project_id": request.PathValue("project_id")})
		return
	}
	value, err := s.projects.GetContext(request.Context(), requestContext.Actor, contract.ProjectID(request.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (s *Server) knowledgeDocumentEntry(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(strings.ToLower(request.Header.Get("Content-Type")), "multipart/") {
		s.createKnowledgeDocument(writer, request)
		return
	}
	s.importKnowledgeDocument(writer, request)
}

func (s *Server) createKnowledgeDocument(writer http.ResponseWriter, request *http.Request) {
	requestContext, _ := contract.RequestContextFrom(request.Context())
	if s.knowledge == nil {
		s.notImplemented(writer, request)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, knowledge.MaxDocumentBytes+1024*1024)
	if err := request.ParseMultipartForm(knowledge.MaxDocumentBytes); err != nil {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_DOCUMENT", Message: "文档上传格式无效或文件过大",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	file, header, err := request.FormFile("file")
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_DOCUMENT", Message: "必须提供名为 file 的 .md 或 .docx 文件",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	defer file.Close()
	value, err := s.knowledge.CreateDocument(
		request.Context(), requestContext.Actor, contract.ProjectID(request.PathValue("project_id")),
		header.Filename, header.Header.Get("Content-Type"), file, header.Size,
	)
	if errors.Is(err, knowledge.ErrInvalidDocument) {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_DOCUMENT", Message: "仅支持有效的 .md 或 .docx，单个文件不超过 10MB",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	if err != nil {
		s.writeServiceError(writer, request, err)
		return
	}
	value.ExtractedText = ""
	writeJSON(writer, http.StatusCreated, value)
}

func (s *Server) runKnowledgeResearch(writer http.ResponseWriter, request *http.Request) {
	requestContext, _ := contract.RequestContextFrom(request.Context())
	if s.knowledge == nil {
		s.notImplemented(writer, request)
		return
	}
	var body knowledge.ResearchRequest
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_REQUEST", Message: "研究请求格式无效",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_REQUEST", Message: "研究请求只能包含一个 JSON 对象",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	value, err := s.knowledge.RunResearch(
		request.Context(), requestContext.Actor, contract.ProjectID(request.PathValue("project_id")), body,
	)
	if errors.Is(err, knowledge.ErrExternalConfirmationRequired) {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code:      "EXTERNAL_CONFIRMATION_REQUIRED",
			Message:   "每次联网搜索或 MCP 调用都必须由当前用户明确确认披露范围",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	if errors.Is(err, knowledge.ErrInvalidResearchRequest) {
		writeProblem(writer, http.StatusBadRequest, contract.Error{
			Code: "INVALID_RESEARCH_REQUEST", Message: "研究请求或披露范围与实际发送内容不一致",
			RequestID: requestContext.RequestID, Retryable: false,
		})
		return
	}
	if err != nil {
		s.writeServiceError(writer, request, err)
		return
	}
	status := http.StatusCreated
	if value.Status == "running" {
		status = http.StatusAccepted
	}
	writeJSON(writer, status, value)
}

type modelJobCreateBody struct {
	Capability            string          `json:"capability"`
	ModelAlias            string          `json:"model_alias"`
	Input                 json.RawMessage `json:"input"`
	ProjectContextVersion int64           `json:"project_context_version"`
	SourceSystem          string          `json:"source_system"`
	SourceTaskID          string          `json:"source_task_id"`
}

func (s *Server) createImageJob(writer http.ResponseWriter, request *http.Request) {
	requestContext, _ := contract.RequestContextFrom(request.Context())
	if s.providerJobs == nil || s.projects == nil {
		s.notImplemented(writer, request)
		return
	}
	var body modelJobCreateBody
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Request body must be one valid model job object", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Request body must be one valid model job object", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	var imageInput provider.ImageGenerationInput
	var videoInput provider.VideoGenerationInput
	switch {
	case supportedImageCapability(body.Capability):
		if err := decodeStrictInput(body.Input, &imageInput); err != nil || imageInput.Validate() != nil {
			writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Image job input is invalid", RequestID: requestContext.RequestID, Retryable: false})
			return
		}
	case body.Capability == "video.generate":
		if err := decodeStrictInput(body.Input, &videoInput); err != nil || videoInput.Validate() != nil {
			writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Video job input is invalid", RequestID: requestContext.RequestID, Retryable: false})
			return
		}
	default:
		writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Only image.generate, image.edit, or video.generate is supported", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	if strings.TrimSpace(body.ModelAlias) == "" || body.ProjectContextVersion < 1 {
		writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Model alias and project context version are required", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	if !requestContext.Actor.HasScope(provider.ScopeJobCreate) {
		writeProblem(writer, http.StatusForbidden, contract.Error{Code: "PERMISSION_DENIED", Message: "Provider job creation is not permitted", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	project, err := s.projects.GetContext(request.Context(), requestContext.Actor, contract.ProjectID(request.PathValue("project_id")))
	if err != nil {
		s.writeServiceError(writer, request, err)
		return
	}
	if err := project.ValidateBrandBound(); err != nil || project.OrganizationID != requestContext.Actor.OrganizationID || project.ProjectID != contract.ProjectID(request.PathValue("project_id")) {
		writeProblem(writer, http.StatusConflict, contract.Error{Code: "PROJECT_NOT_ACTIVE", Message: "Project must be active and brand-bound for model generation", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	if body.ProjectContextVersion != project.ProjectContextVersion {
		writeProblem(writer, http.StatusConflict, contract.Error{Code: "PROJECT_CONTEXT_STALE", Message: "Project context version is stale", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	key := contract.IdempotencyKey(request.Header.Get("Idempotency-Key"))
	if err := key.Validate(); err != nil {
		writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "IDEMPOTENCY_KEY_INVALID", Message: "A valid Idempotency-Key header is required", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	requestHash, err := contract.CanonicalJSONHash(body)
	if err != nil {
		writeProblem(writer, http.StatusInternalServerError, contract.Error{Code: "REQUEST_CANONICALIZATION_FAILED", Message: "Provider request cannot be processed", RequestID: requestContext.RequestID, Retryable: true})
		return
	}
	var job contract.ProviderJob
	if body.Capability == "video.generate" {
		job, _, err = s.providerJobs.CreateVideoJob(request.Context(), provider.CreateVideoJobRequest{
			Actor: requestContext.Actor, Project: project, IdempotencyKey: key, RequestHash: requestHash,
			ModelAlias: body.ModelAlias, SourceSystem: body.SourceSystem, SourceTaskID: body.SourceTaskID, Input: videoInput,
		})
	} else {
		job, _, err = s.providerJobs.CreateImageJob(request.Context(), provider.CreateImageJobRequest{
			Actor: requestContext.Actor, Project: project, IdempotencyKey: key, RequestHash: requestHash,
			ModelAlias: body.ModelAlias, SourceSystem: body.SourceSystem, SourceTaskID: body.SourceTaskID, Operation: body.Capability, Input: imageInput,
		})
	}
	if errors.Is(err, provider.ErrIdempotencyConflict) {
		writeProblem(writer, http.StatusConflict, contract.Error{Code: "IDEMPOTENCY_CONFLICT", Message: "Idempotency key was reused for a different request", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, contract.Error{Code: "INVALID_REQUEST", Message: "Provider job request is invalid", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func decodeStrictInput(raw json.RawMessage, target any) error {
	if len(raw) == 0 {
		return fmt.Errorf("model input is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("model input must contain one JSON object")
	}
	return nil
}

func supportedImageCapability(capability string) bool {
	return capability == "image.generate" || capability == "image.edit"
}

func (s *Server) getProviderJob(writer http.ResponseWriter, request *http.Request) {
	requestContext, _ := contract.RequestContextFrom(request.Context())
	if s.providerJobs == nil {
		writeProblem(writer, http.StatusServiceUnavailable, contract.Error{Code: "DEPENDENCY_UNAVAILABLE", Message: "Provider service is not configured", RequestID: requestContext.RequestID, Retryable: true})
		return
	}
	job, err := s.providerJobs.GetJob(request.Context(), requestContext.Actor.OrganizationID, contract.ProjectID(request.PathValue("project_id")), request.PathValue("job_id"))
	if errors.Is(err, provider.ErrJobNotFound) {
		writeProblem(writer, http.StatusNotFound, contract.Error{Code: "RESOURCE_NOT_FOUND", Message: "Provider job was not found", RequestID: requestContext.RequestID, Retryable: false})
		return
	}
	if err != nil {
		writeProblem(writer, http.StatusServiceUnavailable, contract.Error{Code: "DEPENDENCY_UNAVAILABLE", Message: "Provider service is unavailable", RequestID: requestContext.RequestID, Retryable: true})
		return
	}
	writeJSON(writer, http.StatusOK, job)
}

func (s *Server) notFound(writer http.ResponseWriter, request *http.Request) {
	writeProblem(writer, http.StatusNotFound, contract.Error{
		Code:      "RESOURCE_NOT_FOUND",
		Message:   "请求的资源不存在",
		RequestID: requestIDFrom(request.Context()),
		Retryable: false,
	})
}

func newRequestID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "req_" + hex.EncodeToString(bytes), nil
}

func validOpaqueID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' {
			continue
		}
		return ""
	}
	return value
}

func traceID(traceparentHeader, fallback string) string {
	parts := strings.Split(traceparentHeader, "-")
	if len(parts) == 4 && len(parts[1]) == 32 && isHex(parts[1]) {
		return strings.ToLower(parts[1])
	}
	return fallback
}

func isHex(value string) bool {
	for _, character := range value {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return false
		}
	}
	return true
}

type requestIDKey struct{}

func withRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

func requestIDFrom(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func writeProblem(writer http.ResponseWriter, status int, problem contract.Error) {
	if problem.Details == nil {
		problem.Details = []contract.FieldViolation{}
	}
	writeJSON(writer, status, contract.Problem{Error: problem})
}

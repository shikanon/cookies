package creative

import (
	"context"
	"errors"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

var (
	ErrNotFound             = errors.New("creative resource not found")
	ErrIdempotencyConflict  = errors.New("creative idempotency key conflicts with an earlier request")
	ErrIntakeNotReady       = errors.New("creative intake needs clarification before task creation")
	ErrProviderJobConflict  = errors.New("production job already registered with a different provider job")
	ErrVersionConflict      = errors.New("creative resource version conflict")
	ErrInvalidState         = errors.New("creative resource is not in a state that allows this action")
	ErrProviderInputInvalid = errors.New("creative provider input is invalid")
	ErrFullStrategyRequired = errors.New("the selected creative business requires the full Strategy workflow")
	// Viral analysis failures are intentionally classified at the domain seam so
	// HTTP clients can distinguish a retryable model-gateway issue from an
	// invalid or unreadable source video without receiving provider internals.
	ErrViralAnalysisSourceUnavailable        = errors.New("viral analysis source video is unavailable")
	ErrViralAnalysisPreparationFailed        = errors.New("viral analysis video preparation failed")
	ErrViralAnalysisProviderUnavailable      = errors.New("viral analysis provider is unavailable")
	ErrViralAnalysisProviderRejected         = errors.New("viral analysis provider rejected the request")
	ErrViralAnalysisResponseInvalid          = errors.New("viral analysis provider returned an invalid response")
	ErrShortDramaAnalysisSourceUnavailable   = errors.New("short drama analysis source video is unavailable")
	ErrShortDramaAnalysisPreparationFailed   = errors.New("short drama analysis video preparation failed")
	ErrShortDramaAnalysisProviderUnavailable = errors.New("short drama analysis provider is unavailable")
	ErrShortDramaAnalysisProviderRejected    = errors.New("short drama analysis provider rejected the request")
	ErrShortDramaAnalysisResponseInvalid     = errors.New("short drama analysis provider returned an invalid response")
)

type Repository interface {
	CreateIntake(context.Context, CreativeIntake) (CreativeIntake, bool, error)
	UpdateIntakeReadiness(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, IntakeStatus, []string, string, time.Time) (CreativeIntake, error)
	ListIntakes(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]CreativeIntake, error)
	GetIntake(context.Context, contract.OrganizationID, contract.ProjectID, string) (CreativeIntake, error)
	CreateTask(context.Context, CreativeTask, ImageTextDraft) (CreativeTask, error)
	CreateVideoTask(context.Context, CreativeTask, VideoDraft) (CreativeTask, error)
	ListTasks(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]CreativeTask, error)
	GetTaskDetail(context.Context, contract.OrganizationID, contract.ProjectID, string) (TaskDetail, error)
	CreateShortDramaGenerationAttempt(context.Context, contract.OrganizationID, contract.ProjectID, ShortDramaGenerationAttempt) (ShortDramaGenerationAttempt, error)
	CreateGamePrerollGenerationAttempt(context.Context, contract.OrganizationID, contract.ProjectID, GamePrerollGenerationAttempt) (GamePrerollGenerationAttempt, error)
	ArchiveTask(context.Context, contract.OrganizationID, contract.ProjectID, string, time.Time) error
	ReviseDraft(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, ImageTextDraft) (ImageTextDraft, error)
	RegisterProductionJob(context.Context, contract.OrganizationID, contract.ProjectID, string, ProductionJob) error
	CreateRenderJob(context.Context, RenderJob) (RenderJob, bool, error)
	GetRenderJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (RenderJob, error)
	MarkRenderRunning(context.Context, contract.OrganizationID, contract.ProjectID, string, time.Time) (RenderJob, error)
	CompleteRenderJob(context.Context, contract.OrganizationID, contract.ProjectID, string, contract.ProjectAssetRef, time.Time) error
	FailRenderJob(context.Context, contract.OrganizationID, contract.ProjectID, string, string, string, time.Time) error
	CreateVersion(context.Context, CreativeVersion) (CreativeVersion, bool, error)
	GetVersion(context.Context, contract.OrganizationID, contract.ProjectID, string) (CreativeVersion, error)
	ListVersions(context.Context, contract.OrganizationID, contract.ProjectID, string, int) ([]CreativeVersion, error)
	RecordVersionCheck(context.Context, contract.OrganizationID, contract.ProjectID, string, CreativeCheck) (CreativeVersion, error)
	ApproveVersion(context.Context, contract.OrganizationID, contract.ProjectID, string, CreativeApproval) (CreativeVersion, error)
	CreatePackage(context.Context, CreativePackage) (CreativePackage, error)
	ListPackages(context.Context, contract.OrganizationID, contract.ProjectID, int) ([]CreativePackage, error)
}

// TaskMetadataRepository is the persistence seam for user-facing task identity.
// Draft revisions remain append-only and separate from renaming the containing task.
type TaskMetadataRepository interface {
	RenameTask(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, string, time.Time) (CreativeTask, error)
}

// ViralRemakeRepository is a narrow seam for append-only video-draft
// revisions. It is separate from the base Repository so other Creative test
// adapters do not need to understand the viral-remake workflow.
type ViralRemakeRepository interface {
	ReviseVideoDraft(context.Context, contract.OrganizationID, contract.ProjectID, string, int64, VideoDraft, TaskStatus) (VideoDraft, error)
}

package creativeprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type imageAttemptFinder interface {
	FindImageGenerationAttemptByProviderJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (creative.ImageGenerationAttempt, error)
}

type imageSlotRetryCreative interface {
	GetTaskDetail(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error)
	PrepareImageSlotGeneration(context.Context, contract.RequestContext, contract.ProjectID, string, int, creative.PrepareImageSlotRequest, contract.IdempotencyKey) (creative.ImagePromptPackage, creative.ImageGenerationAttempt, bool, error)
	AttachImageProviderJob(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.ImageGenerationAttempt, error)
	FailImageGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string, string) (creative.ImageGenerationAttempt, error)
}

type imageJobCreator interface {
	CreateImageJob(context.Context, provider.CreateImageJobRequest) (contract.ProviderJob, bool, error)
}

type projectContextReader interface {
	GetContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error)
}

// ImageSlotProductionRetryAdapter re-enters the existing image-slot owner
// workflow. It never reconstructs a Provider request from the Production
// Center projection and never creates a bare Provider job.
type ImageSlotProductionRetryAdapter struct {
	Creative imageSlotRetryCreative
	Attempts imageAttemptFinder
	Provider imageJobCreator
	Projects projectContextReader
}

func (a ImageSlotProductionRetryAdapter) Supports(run creative.ProductionSourceRun) bool {
	return a.Creative != nil && a.Attempts != nil && a.Provider != nil && a.Projects != nil &&
		run.Summary.Ref.Source == creative.ProductionSourceProvider && run.OwnerSystem == "creative" &&
		run.PromptRef != nil && run.PromptRef.Type == "creative_image_prompt_package" &&
		run.Summary.Error != nil && run.Summary.Error.Retryable
}

func (a ImageSlotProductionRetryAdapter) Retry(ctx context.Context, command creative.ProductionRetryContext) (creative.ProductionRunRef, error) {
	previous, err := a.Attempts.FindImageGenerationAttemptByProviderJob(ctx, command.RequestContext.Actor.OrganizationID, command.ProjectID, command.Run.Summary.Ref.ID)
	if err != nil {
		return creative.ProductionRunRef{}, creative.ErrProductionRetryRequiresSourceWorkflow
	}
	detail, err := a.Creative.GetTaskDetail(ctx, command.RequestContext.Actor, command.ProjectID, previous.TaskID)
	if err != nil {
		return creative.ProductionRunRef{}, err
	}
	if detail.Draft.Version != previous.DraftRevision {
		return creative.ProductionRunRef{}, creative.ErrProductionRetryRequiresSourceWorkflow
	}
	prompt, attempt, replay, err := a.Creative.PrepareImageSlotGeneration(ctx, command.RequestContext, command.ProjectID, previous.TaskID, previous.ImagePlanOrder, creative.PrepareImageSlotRequest{
		ExpectedTaskVersion: detail.Task.Version, DraftRevision: previous.DraftRevision, ModelAlias: previous.GenerationSpec.ModelAlias,
	}, command.IdempotencyKey)
	if err != nil {
		return creative.ProductionRunRef{}, err
	}
	if replay && strings.TrimSpace(attempt.ProviderJobID) != "" {
		return creative.ProductionRunRef{Source: creative.ProductionSourceProvider, ID: attempt.ProviderJobID}, nil
	}
	project, err := a.Projects.GetContext(ctx, command.RequestContext.Actor, command.ProjectID)
	if err != nil {
		return creative.ProductionRunRef{}, err
	}
	sourceAssets := make([]contract.ProjectAssetRef, 0, len(prompt.SourceAssetRefs))
	for _, ref := range prompt.SourceAssetRefs {
		sourceAssets = append(sourceAssets, contract.ProjectAssetRef{ProjectID: command.ProjectID, AssetVersion: ref})
	}
	operation := "image.generate"
	if len(sourceAssets) > 0 {
		operation = "image.edit"
	}
	draftRevision := attempt.DraftRevision
	job, _, err := a.Provider.CreateImageJob(ctx, provider.CreateImageJobRequest{
		Actor: command.RequestContext.Actor, Project: project, IdempotencyKey: command.IdempotencyKey, RequestHash: attempt.RequestHash,
		ModelAlias: attempt.GenerationSpec.ModelAlias, SourceSystem: "creative", SourceTaskID: previous.TaskID, Operation: operation,
		Input: provider.ImageGenerationInput{
			Prompt: prompt.CompiledPrompt, Width: attempt.GenerationSpec.Width, Height: attempt.GenerationSpec.Height, SourceAssets: sourceAssets,
			PromptRef:          &contract.ResourceRef{Type: "creative_image_prompt_package", ID: prompt.ID},
			SourceResourceRefs: []contract.ResourceRef{{Type: "creative_task", ID: previous.TaskID}, {Type: "creative_image_text_draft", ID: previous.TaskID, Version: &draftRevision}, {Type: "creative_direction", ID: prompt.DirectionID}},
		},
	})
	if err != nil {
		_, _ = a.Creative.FailImageGenerationAttempt(ctx, command.RequestContext.Actor, command.ProjectID, attempt.ID, "PROVIDER_JOB_CREATE_FAILED", err.Error())
		return creative.ProductionRunRef{}, err
	}
	attached, err := a.Creative.AttachImageProviderJob(ctx, command.RequestContext.Actor, command.ProjectID, attempt.ID, job.ID)
	if err != nil {
		return creative.ProductionRunRef{}, err
	}
	if attached.ProviderJobID != job.ID {
		return creative.ProductionRunRef{}, errors.New("image retry attached an unexpected provider job")
	}
	if strings.TrimSpace(job.ID) == "" {
		return creative.ProductionRunRef{}, fmt.Errorf("image retry provider job ID is empty")
	}
	return creative.ProductionRunRef{Source: creative.ProductionSourceProvider, ID: job.ID}, nil
}

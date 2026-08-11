package creativeprovider

import (
	"context"
	"testing"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type imageRetryCreativeStub struct{}

func (*imageRetryCreativeStub) GetTaskDetail(context.Context, contract.ActorContext, contract.ProjectID, string) (creative.TaskDetail, error) {
	return creative.TaskDetail{}, nil
}
func (*imageRetryCreativeStub) PrepareImageSlotGeneration(context.Context, contract.RequestContext, contract.ProjectID, string, int, creative.PrepareImageSlotRequest, contract.IdempotencyKey) (creative.ImagePromptPackage, creative.ImageGenerationAttempt, bool, error) {
	return creative.ImagePromptPackage{}, creative.ImageGenerationAttempt{}, false, nil
}
func (*imageRetryCreativeStub) AttachImageProviderJob(context.Context, contract.ActorContext, contract.ProjectID, string, string) (creative.ImageGenerationAttempt, error) {
	return creative.ImageGenerationAttempt{}, nil
}
func (*imageRetryCreativeStub) FailImageGenerationAttempt(context.Context, contract.ActorContext, contract.ProjectID, string, string, string) (creative.ImageGenerationAttempt, error) {
	return creative.ImageGenerationAttempt{}, nil
}

type imageAttemptFinderStub struct{}

func (*imageAttemptFinderStub) FindImageGenerationAttemptByProviderJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (creative.ImageGenerationAttempt, error) {
	return creative.ImageGenerationAttempt{}, nil
}

type imageJobCreatorStub struct{}

func (*imageJobCreatorStub) CreateImageJob(context.Context, provider.CreateImageJobRequest) (contract.ProviderJob, bool, error) {
	return contract.ProviderJob{}, false, nil
}

type projectContextReaderStub struct{}

func (*projectContextReaderStub) GetContext(context.Context, contract.ActorContext, contract.ProjectID) (contract.ProjectContext, error) {
	return contract.ProjectContext{}, nil
}

func TestImageSlotProductionRetryAdapterOnlySupportsRetryableOwnedImageSlots(t *testing.T) {
	adapter := ImageSlotProductionRetryAdapter{Creative: &imageRetryCreativeStub{}, Attempts: &imageAttemptFinderStub{}, Provider: &imageJobCreatorStub{}, Projects: &projectContextReaderStub{}}
	prompt := &contract.ResourceRef{Type: "creative_image_prompt_package", ID: "prompt-1"}
	run := creative.ProductionSourceRun{OwnerSystem: "creative", PromptRef: prompt, Summary: creative.ProductionRunSummary{
		Ref:              creative.ProductionRunRef{Source: creative.ProductionSourceProvider, ID: "job-1"},
		NormalizedStatus: creative.ProductionFailed, Error: &creative.ProductionErrorView{Code: "PROVIDER_TIMEOUT", Retryable: true},
	}}
	if !adapter.Supports(run) {
		t.Fatal("owned retryable image slot was not supported")
	}
	run.OwnerSystem = "creative.ai-native-ad"
	if adapter.Supports(run) {
		t.Fatal("AI Native provider job bypassed its source workflow")
	}
	run.OwnerSystem = "creative"
	run.Summary.Error.Retryable = false
	if adapter.Supports(run) {
		t.Fatal("non-retryable provider failure was advertised")
	}
}

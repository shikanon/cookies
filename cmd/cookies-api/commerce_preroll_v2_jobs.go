package main

import (
	"context"
	"fmt"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type creativeCommercePrerollV2ImageJobs struct{ provider *provider.Service }

func (j creativeCommercePrerollV2ImageJobs) CreateCommerceFirstFrameJob(ctx context.Context, actor contract.ActorContext, project contract.ProjectContext, request creative.CommercePrerollV2FirstFrameJobRequest) (contract.ProviderJob, error) {
	if j.provider == nil {
		return contract.ProviderJob{}, fmt.Errorf("commerce preroll image provider is unavailable")
	}
	providerActor := actor
	if !providerActor.HasScope(provider.ScopeJobCreate) {
		providerActor.Scopes = append(providerActor.Scopes, provider.ScopeJobCreate)
	}
	hash, err := contract.CanonicalJSONHash(struct {
		TaskID         string                   `json:"task_id"`
		BatchID        string                   `json:"batch_id"`
		CandidateID    string                   `json:"candidate_id"`
		VariantIndex   int                      `json:"variant_index"`
		Prompt         string                   `json:"prompt"`
		ReferenceAsset contract.ProjectAssetRef `json:"reference_asset"`
	}{request.TaskID, request.BatchID, request.CandidateID, request.VariantIndex, request.Prompt, request.ReferenceAsset})
	if err != nil {
		return contract.ProviderJob{}, err
	}
	job, _, err := j.provider.CreateImageJob(ctx, provider.CreateImageJobRequest{
		Actor: providerActor, Project: project,
		IdempotencyKey: contract.IdempotencyKey("commerce-preroll-v2-frame-" + hash), RequestHash: hash,
		ModelAlias: "cookies.image.standard", SourceSystem: "creative.commerce_preroll_v2", SourceTaskID: request.TaskID,
		Operation: "image.generate",
		Input: provider.ImageGenerationInput{
			Prompt: request.Prompt, Width: request.Width, Height: request.Height,
		},
	})
	return job, err
}

var _ creative.CommercePrerollV2ImageJobCreator = creativeCommercePrerollV2ImageJobs{}

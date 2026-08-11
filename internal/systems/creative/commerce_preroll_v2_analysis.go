package creative

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/cookies/internal/platform/contract"
)

type CommercePrerollV2AnalysisResult struct {
	InputHash     string                               `json:"input_hash"`
	PromptVersion string                               `json:"prompt_version"`
	Content       CommercePrerollV2SourceUnderstanding `json:"content"`
}

type CommercePrerollV2Analyzer interface {
	Analyze(context.Context, contract.ActorContext, contract.ProjectContext, contract.ProjectAssetRef) (CommercePrerollV2AnalysisResult, error)
}

type AnalyzeCommercePrerollV2SourceRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type ConfirmCommercePrerollV2UnderstandingRequest struct {
	ExpectedRevision int64                         `json:"expected_revision"`
	Product          CommercePrerollV2ProductFacts `json:"product"`
	AcceptedRisks    []string                      `json:"accepted_risks"`
}

func (s Service) AnalyzeCommercePrerollV2Source(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request AnalyzeCommercePrerollV2SourceRequest,
) (TaskDetail, error) {
	if s.CommercePrerollV2Analyzer == nil {
		return TaskDetail{}, fmt.Errorf("commerce preroll V2 analysis capability is unavailable")
	}
	project, err := s.Projects.RequireActiveContext(ctx, actor, projectID)
	if err != nil {
		return TaskDetail{}, err
	}
	detail, err := s.GetCommercePrerollV2Workspace(ctx, actor, projectID, taskID)
	if err != nil {
		return TaskDetail{}, err
	}
	if detail.VideoDraft.Revision != request.ExpectedRevision {
		return TaskDetail{}, ErrVersionConflict
	}
	result, err := s.CommercePrerollV2Analyzer.Analyze(ctx, actor, project, detail.VideoDraft.CommercePrerollV2.SourceVideo)
	if err != nil {
		return TaskDetail{}, err
	}
	if err := validateCommercePrerollV2AnalysisResult(result); err != nil {
		return TaskDetail{}, err
	}
	sourceDurationMS := detail.VideoDraft.CommercePrerollV2.SourceMetadata.DurationMS
	if sourceDurationMS > 0 && (result.Content.ProductFrameMS >= sourceDurationMS || result.Content.OpeningAnchorFrameMS >= sourceDurationMS) {
		return TaskDetail{}, fmt.Errorf("commerce preroll V2 analysis frame timestamp exceeds the source duration")
	}
	for _, candidate := range result.Content.ProductFrameCandidates {
		if sourceDurationMS > 0 && candidate.TimestampMS >= sourceDurationMS {
			return TaskDetail{}, fmt.Errorf("commerce product candidate timestamp exceeds the source duration")
		}
	}
	if result.Content.OpeningAnchorFrameMS > 3000 {
		return TaskDetail{}, fmt.Errorf("commerce preroll V2 opening anchor must come from the first three seconds")
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			workspace.ActiveStage = CommercePrerollV2StageUnderstandingReady
			workspace.Analysis = CommercePrerollV2Analysis{
				CommercePrerollV2AsyncResource: CommercePrerollV2AsyncResource{Status: CommercePrerollV2ResourceReady, Progress: 100},
				Revision:                       1, InputHash: result.InputHash, PromptVersion: result.PromptVersion, Content: result.Content,
			}
			draft.Prompt = "原视频理解已完成，等待确认商品事实"
			return nil
		}, TaskInProgress)
}

func (s Service) ConfirmCommercePrerollV2Understanding(
	ctx context.Context,
	actor contract.ActorContext,
	projectID contract.ProjectID,
	taskID string,
	request ConfirmCommercePrerollV2UnderstandingRequest,
) (TaskDetail, error) {
	if err := validateCommercePrerollV2ProductFacts(request.Product); err != nil {
		return TaskDetail{}, err
	}
	return s.reviseCommercePrerollV2(ctx, actor, projectID, taskID, request.ExpectedRevision,
		func(draft *VideoDraft, workspace *CommercePrerollV2Workspace) error {
			if workspace.ActiveStage != CommercePrerollV2StageUnderstandingReady || workspace.Analysis.Status != CommercePrerollV2ResourceReady {
				return ErrInvalidState
			}
			workspace.Analysis.Revision++
			workspace.Analysis.Content.Product = request.Product
			workspace.Analysis.Content.Risks = append([]string{}, request.AcceptedRisks...)
			workspace.ActiveStage = CommercePrerollV2StageUnderstandingConfirmed
			draft.Prompt = "商品事实已确认，等待生成钩子"
			return nil
		}, TaskInProgress)
}

func validateCommercePrerollV2AnalysisResult(result CommercePrerollV2AnalysisResult) error {
	if strings.TrimSpace(result.InputHash) == "" || strings.TrimSpace(result.PromptVersion) == "" ||
		strings.TrimSpace(result.Content.VisualStyle) == "" || strings.TrimSpace(result.Content.OpeningShot) == "" ||
		result.Content.ProductFrameMS < 0 || result.Content.OpeningAnchorFrameMS < 0 || len(result.Content.Evidence) == 0 {
		return fmt.Errorf("commerce preroll V2 analysis result is incomplete")
	}
	if err := validateCommercePrerollV2ProductFacts(result.Content.Product); err != nil {
		return err
	}
	for _, evidence := range result.Content.Evidence {
		if strings.TrimSpace(evidence.ID) == "" || strings.TrimSpace(evidence.Source) == "" ||
			strings.TrimSpace(evidence.Excerpt) == "" || evidence.TimestampMS < 0 || evidence.Confidence < 0 || evidence.Confidence > 1 {
			return fmt.Errorf("commerce preroll V2 evidence is incomplete")
		}
	}
	for _, candidate := range result.Content.ProductFrameCandidates {
		if strings.TrimSpace(candidate.ID) == "" || candidate.TimestampMS < 0 || candidate.Frontality < 0 || candidate.Frontality > 1 || candidate.Sharpness < 0 || candidate.Sharpness > 1 || candidate.Completeness < 0 || candidate.Completeness > 1 || candidate.LogoReadability < 0 || candidate.LogoReadability > 1 || candidate.Occlusion < 0 || candidate.Occlusion > 1 || candidate.Overall < 0 || candidate.Overall > 1 {
			return fmt.Errorf("commerce product frame candidate is incomplete")
		}
	}
	return nil
}

func validateCommercePrerollV2ProductFacts(product CommercePrerollV2ProductFacts) error {
	if strings.TrimSpace(product.Name) == "" || strings.TrimSpace(product.Category) == "" ||
		strings.TrimSpace(product.Description) == "" || len(product.SellingPoints) == 0 ||
		len(product.AppearanceGuardrails) == 0 {
		return fmt.Errorf("commerce preroll V2 product facts are incomplete")
	}
	return nil
}

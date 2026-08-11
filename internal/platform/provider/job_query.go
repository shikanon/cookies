package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const maxJobQueryLimit = 100

// JobQuery is Provider's read-only, Project-scoped seam for consumers such as
// Creative's production center. It deliberately exposes a safe projection,
// never Provider credentials, prompt text, external IDs, or temporary URLs.
type JobQuery interface {
	ListCreativeJobs(context.Context, JobQueryRequest) (JobQueryPage, error)
	GetCreativeJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (JobQueryItem, error)
}

type JobQueryRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	Statuses       []contract.ProviderJobStatus
	MediaKind      string
	SourceTaskID   string
	CreatedAfter   *time.Time
	CreatedBefore  *time.Time
	Query          string
	BeforeCreated  *time.Time
	BeforeID       string
	Limit          int
}

type JobQueryFilter struct {
	JobQueryRequest
	CreativeOnly bool
}

type JobQueryStore interface {
	ListJobs(context.Context, JobQueryFilter) ([]JobRecord, bool, error)
}

type JobQueryItem struct {
	Job            contract.ProviderJob       `json:"job"`
	Operation      string                     `json:"operation"`
	ModelAlias     string                     `json:"model_alias"`
	ActualModel    *string                    `json:"actual_model"`
	SourceSystem   string                     `json:"source_system"`
	SourceTaskID   string                     `json:"source_task_id"`
	InputAssetRefs []contract.AssetVersionRef `json:"input_asset_refs"`
	PromptRef      *contract.ResourceRef      `json:"prompt_ref"`
	Parameters     map[string]any             `json:"parameters"`
	OutputCount    int                        `json:"output_count"`
	Usage          *JobUsage                  `json:"usage"`
	Events         []JobEvent                 `json:"events"`
}

type JobQueryPage struct {
	Items   []JobQueryItem `json:"items"`
	HasMore bool           `json:"has_more"`
}

func (s Service) ListCreativeJobs(ctx context.Context, request JobQueryRequest) (JobQueryPage, error) {
	if s.JobQueryStore == nil {
		return JobQueryPage{}, fmt.Errorf("provider job query store is required")
	}
	if strings.TrimSpace(string(request.OrganizationID)) == "" || strings.TrimSpace(string(request.ProjectID)) == "" {
		return JobQueryPage{}, fmt.Errorf("organization_id and project_id are required")
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > maxJobQueryLimit {
		return JobQueryPage{}, fmt.Errorf("provider job query limit must be between 1 and %d", maxJobQueryLimit)
	}
	records, hasMore, err := s.JobQueryStore.ListJobs(ctx, JobQueryFilter{JobQueryRequest: request, CreativeOnly: true})
	if err != nil {
		return JobQueryPage{}, err
	}
	items := make([]JobQueryItem, 0, len(records))
	for _, record := range records {
		items = append(items, safeJobQueryItem(record))
	}
	return JobQueryPage{Items: items, HasMore: hasMore}, nil
}

func (s Service) GetCreativeJob(ctx context.Context, organizationID contract.OrganizationID, projectID contract.ProjectID, jobID string) (JobQueryItem, error) {
	if s.Store == nil {
		return JobQueryItem{}, fmt.Errorf("provider job store is required")
	}
	record, err := s.Store.Get(ctx, organizationID, projectID, jobID)
	if err != nil {
		return JobQueryItem{}, err
	}
	if record.SourceSystem != "creative" && !strings.HasPrefix(record.SourceSystem, "creative.") {
		return JobQueryItem{}, ErrJobNotFound
	}
	return safeJobQueryItem(record), nil
}

func safeJobQueryItem(record JobRecord) JobQueryItem {
	item := JobQueryItem{
		Job: record.Job, Operation: record.Operation, ModelAlias: record.ModelAlias,
		SourceSystem: record.SourceSystem, SourceTaskID: record.SourceTaskID,
		InputAssetRefs: []contract.AssetVersionRef{}, Parameters: map[string]any{}, OutputCount: len(record.Outputs), Events: []JobEvent{},
	}
	if record.Job.Error != nil {
		item.Job.Error = &contract.JobError{
			Code:      boundedSafeToken(record.Job.Error.Code, 128, "PROVIDER_ERROR"),
			Message:   sanitizeJobEventMessage(record.Job.Error.Message),
			Retryable: record.Job.Error.Retryable,
		}
	}
	if record.Usage != nil {
		usage := *record.Usage
		item.Usage = &usage
	}
	for _, event := range record.Events {
		item.Events = append(item.Events, sanitizeJobEvent(event))
	}
	if strings.TrimSpace(record.ActualModel) != "" {
		value := record.ActualModel
		item.ActualModel = &value
	}
	switch record.Operation {
	case imageGenerateOperation, imageEditOperation:
		for _, ref := range record.Input.SourceAssets {
			item.InputAssetRefs = append(item.InputAssetRefs, ref.AssetVersion)
		}
		item.PromptRef = record.Input.PromptRef
		item.Parameters["width"] = record.Input.Width
		item.Parameters["height"] = record.Input.Height
	case videoGenerateOperation:
		for _, input := range record.VideoInput.ConditioningAssets {
			item.InputAssetRefs = append(item.InputAssetRefs, input.Reference.AssetVersion)
		}
		item.Parameters["duration_seconds"] = record.VideoInput.DurationSeconds
		item.Parameters["aspect_ratio"] = record.VideoInput.AspectRatio
		item.Parameters["resolution"] = record.VideoInput.Resolution
		if record.VideoInput.AudioPolicy != "" {
			item.Parameters["audio_policy"] = record.VideoInput.AudioPolicy
		}
	}
	return item
}

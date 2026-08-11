package creative

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/provider"
)

type ProviderRunAdapter struct{ Jobs provider.JobQuery }

func (ProviderRunAdapter) Source() ProductionRunSourceKind { return ProductionSourceProvider }

func (a ProviderRunAdapter) List(ctx context.Context, scope ProductionSourceScope) (ProductionSourcePage, error) {
	if a.Jobs == nil {
		return ProductionSourcePage{}, errors.New("provider job query is required")
	}
	statuses := providerStatusesForProduction(scope.Statuses)
	if scope.MediaKind == ProductionMediaRender || scope.MediaKind == ProductionMediaAudio {
		return ProductionSourcePage{Items: []ProductionSourceRun{}}, nil
	}
	page, err := a.Jobs.ListCreativeJobs(ctx, provider.JobQueryRequest{
		OrganizationID: scope.OrganizationID, ProjectID: scope.ProjectID, Statuses: statuses,
		MediaKind: string(scope.MediaKind), SourceTaskID: scope.SourceTaskID, CreatedAfter: scope.CreatedAfter, CreatedBefore: scope.CreatedBefore, Query: scope.Query,
		BeforeCreated: scope.BeforeCreated, BeforeID: nativeIDFromWatermark(scope.BeforeKey, ProductionSourceProvider), Limit: sourceQueryLimit(scope.Limit),
	})
	if err != nil {
		return ProductionSourcePage{}, err
	}
	items := make([]ProductionSourceRun, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, providerProductionRun(item))
	}
	return ProductionSourcePage{Items: items, HasMore: page.HasMore}, nil
}

func (a ProviderRunAdapter) Get(ctx context.Context, key ProductionSourceKey) (ProductionSourceRun, error) {
	if a.Jobs == nil {
		return ProductionSourceRun{}, errors.New("provider job query is required")
	}
	item, err := a.Jobs.GetCreativeJob(ctx, key.OrganizationID, key.ProjectID, key.ID)
	if errors.Is(err, provider.ErrJobNotFound) {
		return ProductionSourceRun{}, ErrProductionRunNotFound
	}
	if err != nil {
		return ProductionSourceRun{}, err
	}
	return providerProductionRun(item), nil
}

func providerProductionRun(item provider.JobQueryItem) ProductionSourceRun {
	execution, providerStatus := string(item.Job.ExecutionStatus), string(item.Job.ProviderStatus)
	outputRefs := make([]contract.AssetVersionRef, 0, len(item.Job.ProjectAssetRefs))
	for _, ref := range item.Job.ProjectAssetRefs {
		outputRefs = append(outputRefs, ref.AssetVersion)
	}
	var sourceTask *ProductionSourceTask
	if strings.TrimSpace(item.SourceTaskID) != "" {
		sourceTask = &ProductionSourceTask{System: "creative", ObjectType: "creative_task", ObjectID: item.SourceTaskID}
	}
	var model *ProductionModelView
	if item.ModelAlias != "" {
		model = &ProductionModelView{LogicalAlias: item.ModelAlias, ActualModel: item.ActualModel, Degraded: false}
	}
	var failure *ProductionErrorView
	if item.Job.Error != nil {
		failure = &ProductionErrorView{Code: item.Job.Error.Code, Message: item.Job.Error.Message, Retryable: item.Job.Error.Retryable}
	}
	cost := providerProductionCost(item.Usage)
	events := make([]ProductionRunEvent, 0, len(item.Events))
	for _, event := range item.Events {
		var errorCode *string
		if event.ErrorCode != "" {
			value := event.ErrorCode
			errorCode = &value
		}
		events = append(events, ProductionRunEvent{
			Ordinal: event.Ordinal, Stage: event.Stage, SafeMessage: event.SafeMessage,
			ErrorCode: errorCode, OccurredAt: event.OccurredAt,
		})
	}
	return ProductionSourceRun{
		OwnerSystem: item.SourceSystem,
		Summary: ProductionRunSummary{
			Ref: ProductionRunRef{Source: ProductionSourceProvider, ID: item.Job.ID}, ProjectID: item.Job.ProjectID,
			MediaKind: providerMediaKind(item.Job.Kind), OperationKind: item.Operation, SourceTask: sourceTask,
			NormalizedStatus: normalizeProviderStatus(item.Job.ProviderStatus),
			NativeStatus:     ProductionNativeStatus{Family: "provider", ExecutionStatus: &execution, ProviderStatus: &providerStatus},
			ProgressPercent:  item.Job.Progress, Model: model, OutputCount: item.OutputCount, Cost: cost, Error: failure,
			Actions: ProductionActions{Retry: false, OpenSource: sourceTask != nil}, CreatedAt: item.Job.CreatedAt, UpdatedAt: item.Job.UpdatedAt,
		},
		InputAssetRefs: nonNilAssetRefs(item.InputAssetRefs), OutputAssetRefs: outputRefs,
		Parameters: nonNilParameters(item.Parameters), PromptRef: item.PromptRef,
		Attempt: ProductionAttempt{AttemptCount: item.Job.AttemptCount, MaxAttempts: item.Job.MaxAttempts}, RetryChain: []ProductionRunRef{}, RunEvents: events,
	}
}

func providerProductionCost(usage *provider.JobUsage) *ProductionCostView {
	if usage == nil {
		reason := "Provider usage has not been recorded for this job."
		return &ProductionCostView{Availability: "unavailable", Currency: "CNY", UnavailableReason: &reason}
	}
	if usage.ActualCostMinor == nil {
		reason := "Provider actual cost has not been reported."
		return &ProductionCostView{Availability: "unavailable", Currency: usage.Currency, UnavailableReason: &reason}
	}
	amount := *usage.ActualCostMinor
	return &ProductionCostView{Availability: "actual", Currency: usage.Currency, AmountMinor: &amount}
}

type CreativeRenderJobQuery interface {
	ListProductionRenderJobs(context.Context, ProductionSourceScope) ([]RenderJob, bool, error)
	GetRenderJob(context.Context, contract.OrganizationID, contract.ProjectID, string) (RenderJob, error)
}

type CreativeRenderRunAdapter struct{ Jobs CreativeRenderJobQuery }

func (CreativeRenderRunAdapter) Source() ProductionRunSourceKind {
	return ProductionSourceCreativeRender
}
func (a CreativeRenderRunAdapter) List(ctx context.Context, scope ProductionSourceScope) (ProductionSourcePage, error) {
	if a.Jobs == nil {
		return ProductionSourcePage{}, errors.New("creative render query is required")
	}
	jobs, more, err := a.Jobs.ListProductionRenderJobs(ctx, scope)
	if err != nil {
		return ProductionSourcePage{}, err
	}
	items := make([]ProductionSourceRun, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, creativeRenderProductionRun(job))
	}
	return ProductionSourcePage{Items: items, HasMore: more}, nil
}
func (a CreativeRenderRunAdapter) Get(ctx context.Context, key ProductionSourceKey) (ProductionSourceRun, error) {
	job, err := a.Jobs.GetRenderJob(ctx, key.OrganizationID, key.ProjectID, key.ID)
	if errors.Is(err, ErrNotFound) {
		return ProductionSourceRun{}, ErrProductionRunNotFound
	}
	if err != nil {
		return ProductionSourceRun{}, err
	}
	return creativeRenderProductionRun(job), nil
}

func creativeRenderProductionRun(job RenderJob) ProductionSourceRun {
	renderStatus := string(job.Status)
	inputs := []contract.AssetVersionRef{job.PreRollVideo, job.MainVideo}
	outputs := []contract.AssetVersionRef{}
	if job.OutputAsset != nil {
		outputs = append(outputs, job.OutputAsset.AssetVersion)
	}
	var failure *ProductionErrorView
	if job.ErrorCode != "" {
		failure = &ProductionErrorView{Code: safeRenderErrorCode(job.ErrorCode), Message: safeRenderErrorMessage(job.ErrorMessage), Retryable: false}
	}
	cost, events := renderOwnerFacts("Creative render", string(job.Status), job.ErrorCode, job.CreatedAt, job.UpdatedAt)
	if job.ProductionUsage != nil {
		cost = renderProductionCost(*job.ProductionUsage)
	}
	if len(job.ProductionEvents) > 0 {
		events = renderProductionEvents(job.ProductionEvents)
	}
	return ProductionSourceRun{
		Summary: ProductionRunSummary{
			Ref: ProductionRunRef{Source: ProductionSourceCreativeRender, ID: job.ID}, ProjectID: job.ProjectID,
			MediaKind: ProductionMediaRender, OperationKind: "video.preroll.compose",
			SourceTask:       &ProductionSourceTask{System: "creative", ObjectType: "creative_task", ObjectID: job.TaskID},
			NormalizedStatus: normalizeRenderStatus(string(job.Status)), NativeStatus: ProductionNativeStatus{Family: string(ProductionSourceCreativeRender), RenderStatus: &renderStatus},
			ProgressPercent: renderProgress(string(job.Status), 0), OutputCount: len(outputs), Cost: cost, Error: failure,
			Actions: ProductionActions{OpenSource: true}, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
		},
		InputAssetRefs: inputs, OutputAssetRefs: outputs, Parameters: map[string]any{"render_kind": "final"},
		Attempt: ProductionAttempt{AttemptCount: 0, MaxAttempts: 1}, RetryChain: []ProductionRunRef{}, RunEvents: events,
	}
}

type EditingRenderJobQuery interface {
	ListProductionEditingRenderJobs(context.Context, ProductionSourceScope) ([]EditingRenderJob, bool, error)
	GetEditingRender(context.Context, contract.OrganizationID, contract.ProjectID, string) (EditingRenderJob, error)
}

type EditingRenderRunAdapter struct{ Jobs EditingRenderJobQuery }

func (EditingRenderRunAdapter) Source() ProductionRunSourceKind { return ProductionSourceEditingRender }
func (a EditingRenderRunAdapter) List(ctx context.Context, scope ProductionSourceScope) (ProductionSourcePage, error) {
	if a.Jobs == nil {
		return ProductionSourcePage{}, errors.New("editing render query is required")
	}
	jobs, more, err := a.Jobs.ListProductionEditingRenderJobs(ctx, scope)
	if err != nil {
		return ProductionSourcePage{}, err
	}
	items := make([]ProductionSourceRun, 0, len(jobs))
	for _, job := range jobs {
		items = append(items, editingRenderProductionRun(job))
	}
	return ProductionSourcePage{Items: items, HasMore: more}, nil
}
func (a EditingRenderRunAdapter) Get(ctx context.Context, key ProductionSourceKey) (ProductionSourceRun, error) {
	job, err := a.Jobs.GetEditingRender(ctx, key.OrganizationID, key.ProjectID, key.ID)
	if errors.Is(err, ErrNotFound) {
		return ProductionSourceRun{}, ErrProductionRunNotFound
	}
	if err != nil {
		return ProductionSourceRun{}, err
	}
	return editingRenderProductionRun(job), nil
}

func editingRenderProductionRun(job EditingRenderJob) ProductionSourceRun {
	renderStatus := string(job.Status)
	outputs := []contract.AssetVersionRef{}
	if job.OutputAsset != nil {
		outputs = append(outputs, job.OutputAsset.AssetVersion)
	}
	var failure *ProductionErrorView
	if job.ErrorCode != "" {
		failure = &ProductionErrorView{Code: safeRenderErrorCode(job.ErrorCode), Message: safeRenderErrorMessage(job.ErrorMessage), Retryable: false}
	}
	var retryOf *ProductionRunRef
	retryChain := []ProductionRunRef{}
	if job.RetryOf != "" {
		value := ProductionRunRef{Source: ProductionSourceEditingRender, ID: job.RetryOf}
		retryOf = &value
		retryChain = append(retryChain, value)
	}
	cost, events := renderOwnerFacts("Editing render", string(job.Status), job.ErrorCode, job.CreatedAt, job.UpdatedAt)
	if job.ProductionUsage != nil {
		cost = renderProductionCost(*job.ProductionUsage)
	}
	if len(job.ProductionEvents) > 0 {
		events = renderProductionEvents(job.ProductionEvents)
	}
	return ProductionSourceRun{
		Summary: ProductionRunSummary{
			Ref: ProductionRunRef{Source: ProductionSourceEditingRender, ID: job.ID}, ProjectID: job.ProjectID,
			MediaKind: ProductionMediaRender, OperationKind: "editing.render." + string(job.Kind),
			SourceTask:       &ProductionSourceTask{System: "creative", ObjectType: "edit_task", ObjectID: job.EditTaskID},
			NormalizedStatus: normalizeRenderStatus(string(job.Status)), NativeStatus: ProductionNativeStatus{Family: string(ProductionSourceEditingRender), RenderStatus: &renderStatus},
			ProgressPercent: renderProgress(string(job.Status), job.ProgressPercent), OutputCount: len(outputs), Cost: cost, Error: failure,
			Actions: ProductionActions{OpenSource: true}, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
		},
		InputAssetRefs: timelineAssetRefs(job.Timeline), OutputAssetRefs: outputs, Parameters: map[string]any{"render_kind": string(job.Kind)},
		Attempt: ProductionAttempt{AttemptCount: 0, MaxAttempts: 1, RetryOf: retryOf}, RetryChain: retryChain, RunEvents: events,
	}
}

func renderOwnerFacts(owner, status, errorCode string, createdAt, updatedAt time.Time) (*ProductionCostView, []ProductionRunEvent) {
	reason := owner + " actual cost is not metered."
	cost := &ProductionCostView{Availability: "unavailable", Currency: "CNY", UnavailableReason: &reason}
	events := []ProductionRunEvent{{Ordinal: 1, Stage: "queued", SafeMessage: owner + " queued.", OccurredAt: createdAt}}
	if status == "" || status == "queued" {
		return cost, events
	}
	event := ProductionRunEvent{Ordinal: 2, Stage: safeRenderToken(status, 64), SafeMessage: owner + " state changed to " + safeRenderToken(status, 64) + ".", OccurredAt: updatedAt}
	if code := safeRenderToken(errorCode, 128); code != "" {
		event.ErrorCode = &code
	}
	events = append(events, event)
	return cost, events
}

func renderProductionCost(usage RenderUsage) *ProductionCostView {
	currency := usage.Currency
	if len(currency) != 3 || strings.ToUpper(currency) != currency {
		currency = "CNY"
	}
	if usage.ActualCostMinor != nil && *usage.ActualCostMinor >= 0 {
		amount := *usage.ActualCostMinor
		return &ProductionCostView{Availability: "actual", Currency: currency, AmountMinor: &amount}
	}
	reason := "Creative render actual cost is not available."
	if usage.UnavailableReason != nil && strings.TrimSpace(*usage.UnavailableReason) != "" {
		reason = strings.TrimSpace(*usage.UnavailableReason)
	}
	return &ProductionCostView{Availability: "unavailable", Currency: currency, UnavailableReason: &reason}
}

func renderProductionEvents(ownerEvents []RenderEvent) []ProductionRunEvent {
	events := make([]ProductionRunEvent, 0, len(ownerEvents))
	for _, ownerEvent := range ownerEvents {
		stage := safeRenderToken(ownerEvent.Stage, 64)
		if stage == "" {
			stage = "unknown"
		}
		var errorCode *string
		if code := safeRenderToken(ownerEvent.ErrorCode, 128); code != "" {
			errorCode = &code
		}
		events = append(events, ProductionRunEvent{
			Ordinal: ownerEvent.Ordinal, Stage: stage, SafeMessage: safeRenderEventMessage(ownerEvent.SafeMessage),
			ErrorCode: errorCode, OccurredAt: ownerEvent.OccurredAt,
		})
	}
	return events
}

func safeRenderToken(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.') {
			return ""
		}
	}
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func safeRenderErrorCode(value string) string {
	if code := safeRenderToken(value, 128); code != "" {
		return code
	}
	return "RENDER_ERROR"
}

func safeRenderErrorMessage(value string) string {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	for _, marker := range []string{"authorization", "bearer ", "api_key", "apikey", "secret", "prompt=", "prompt:", "http://", "https://", "s3://", "oss://", "bucket", "object_key", "storage_key"} {
		if strings.Contains(lower, marker) {
			return "Render failed; details were redacted."
		}
	}
	runes := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\t' {
			return -1
		}
		return r
	}, trimmed))
	if len(runes) > 512 {
		runes = runes[:512]
	}
	if len(runes) == 0 {
		return "Render failed."
	}
	return string(runes)
}

func safeRenderEventMessage(value string) string {
	safe := safeRenderErrorMessage(value)
	if safe == "Render failed; details were redacted." {
		return "Render event details were redacted."
	}
	if safe == "Render failed." {
		return "Render state changed."
	}
	return safe
}

func timelineAssetRefs(timeline TimelineVersion) []contract.AssetVersionRef {
	refs := []contract.AssetVersionRef{}
	seen := map[string]bool{}
	appendRef := func(ref *contract.AssetVersionRef) {
		if ref == nil {
			return
		}
		key := string(ref.AssetID) + ":" + fmtInt64(ref.Version)
		if !seen[key] {
			seen[key] = true
			refs = append(refs, *ref)
		}
	}
	if timeline.TimelineV2 != nil {
		for _, track := range timeline.TimelineV2.Tracks {
			for _, clip := range track.Clips {
				appendRef(clip.AssetRef)
			}
		}
		return refs
	}
	for _, track := range timeline.Timeline.Tracks {
		for _, clip := range track.Clips {
			appendRef(clip.AssetRef)
		}
	}
	return refs
}

func normalizeProviderStatus(status contract.ProviderJobStatus) ProductionStatus {
	switch status {
	case contract.ProviderJobSubmitted:
		return ProductionQueued
	case contract.ProviderJobRunning:
		return ProductionRunning
	case contract.ProviderJobOutputsReady, contract.ProviderJobIngesting:
		return ProductionIngesting
	case contract.ProviderJobSucceeded:
		return ProductionSucceeded
	case contract.ProviderJobPartiallySucceeded:
		return ProductionPartiallySucceeded
	case contract.ProviderJobFailed:
		return ProductionFailed
	case contract.ProviderJobCancelled:
		return ProductionCancelled
	case contract.ProviderJobExpired:
		return ProductionExpired
	default:
		return ProductionFailed
	}
}

func normalizeRenderStatus(status string) ProductionStatus {
	switch status {
	case "queued":
		return ProductionQueued
	case "running":
		return ProductionRunning
	case "succeeded":
		return ProductionSucceeded
	case "cancelled":
		return ProductionCancelled
	default:
		return ProductionFailed
	}
}

func providerMediaKind(kind string) ProductionMediaKind {
	if strings.Contains(kind, ".image.") {
		return ProductionMediaImage
	}
	return ProductionMediaVideo
}

func providerStatusesForProduction(statuses []ProductionStatus) []contract.ProviderJobStatus {
	result := []contract.ProviderJobStatus{}
	for _, status := range statuses {
		switch status {
		case ProductionQueued:
			result = append(result, contract.ProviderJobSubmitted)
		case ProductionRunning:
			result = append(result, contract.ProviderJobRunning)
		case ProductionIngesting:
			result = append(result, contract.ProviderJobOutputsReady, contract.ProviderJobIngesting)
		case ProductionSucceeded:
			result = append(result, contract.ProviderJobSucceeded)
		case ProductionPartiallySucceeded:
			result = append(result, contract.ProviderJobPartiallySucceeded)
		case ProductionFailed:
			result = append(result, contract.ProviderJobFailed)
		case ProductionCancelled:
			result = append(result, contract.ProviderJobCancelled)
		case ProductionExpired:
			result = append(result, contract.ProviderJobExpired)
		}
	}
	return result
}

func sourceQueryLimit(limit int) int {
	if limit < 1 {
		return 50
	}
	return limit
}

func nativeIDFromWatermark(key string, source ProductionRunSourceKind) string {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) != 2 {
		return ""
	}
	if string(source) < parts[0] {
		return "~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~"
	}
	if string(source) > parts[0] {
		return ""
	}
	return parts[1]
}

func renderProgress(status string, current int) int {
	if status == "succeeded" {
		return 100
	}
	return current
}

func fmtInt64(value int64) string {
	const digits = "0123456789"
	if value == 0 {
		return "0"
	}
	buffer := [20]byte{}
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = digits[value%10]
		value /= 10
	}
	return string(buffer[index:])
}

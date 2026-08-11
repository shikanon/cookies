package creative

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const productionRunPageContractVersion = "creative-production-run-page/v1"
const productionRunDetailContractVersion = "creative-production-run-detail/v1"
const productionAssetPageContractVersion = "creative-production-asset-page/v1"

var ErrProductionRunNotFound = errors.New("production run not found")
var ErrProductionCursorInvalid = errors.New("production cursor is invalid")
var ErrProductionSourcesUnavailable = errors.New("production sources are unavailable")

type ProductionRunSourceKind string

const (
	ProductionSourceProvider       ProductionRunSourceKind = "provider"
	ProductionSourceCreativeRender ProductionRunSourceKind = "creative_render"
	ProductionSourceEditingRender  ProductionRunSourceKind = "editing_render"
	ProductionSourceAudioRender    ProductionRunSourceKind = "audio_render"
)

type ProductionStatus string

const (
	ProductionQueued             ProductionStatus = "queued"
	ProductionRunning            ProductionStatus = "running"
	ProductionIngesting          ProductionStatus = "ingesting"
	ProductionSucceeded          ProductionStatus = "succeeded"
	ProductionPartiallySucceeded ProductionStatus = "partially_succeeded"
	ProductionFailed             ProductionStatus = "failed"
	ProductionExpired            ProductionStatus = "expired"
	ProductionCancelled          ProductionStatus = "cancelled"
)

type ProductionMediaKind string

const (
	ProductionMediaImage  ProductionMediaKind = "image"
	ProductionMediaVideo  ProductionMediaKind = "video"
	ProductionMediaAudio  ProductionMediaKind = "audio"
	ProductionMediaRender ProductionMediaKind = "render"
)

type ProductionRunRef struct {
	Source ProductionRunSourceKind `json:"source"`
	ID     string                  `json:"id"`
}

func (r ProductionRunRef) Key() string { return string(r.Source) + ":" + r.ID }

type ProductionSourceTask struct {
	System      string  `json:"system"`
	ObjectType  string  `json:"object_type"`
	ObjectID    string  `json:"object_id"`
	DisplayName *string `json:"display_name"`
}

type ProductionNativeStatus struct {
	Family          string  `json:"family"`
	ExecutionStatus *string `json:"execution_status"`
	ProviderStatus  *string `json:"provider_status"`
	RenderStatus    *string `json:"render_status"`
}

type ProductionModelView struct {
	LogicalAlias string  `json:"logical_alias"`
	ActualModel  *string `json:"actual_model"`
	Degraded     bool    `json:"degraded"`
}

type ProductionErrorView struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type ProductionCostView struct {
	Availability      string  `json:"availability"`
	Currency          string  `json:"currency"`
	AmountMinor       *int64  `json:"amount_minor"`
	UnavailableReason *string `json:"unavailable_reason"`
}

type ProductionActions struct {
	Retry      bool `json:"retry"`
	OpenSource bool `json:"open_source"`
}

type ProductionRunSummary struct {
	Ref              ProductionRunRef       `json:"ref"`
	ProjectID        contract.ProjectID     `json:"project_id"`
	MediaKind        ProductionMediaKind    `json:"media_kind"`
	OperationKind    string                 `json:"operation_kind"`
	SourceTask       *ProductionSourceTask  `json:"source_task"`
	NormalizedStatus ProductionStatus       `json:"normalized_status"`
	NativeStatus     ProductionNativeStatus `json:"native_status"`
	ProgressPercent  int                    `json:"progress_percent"`
	Model            *ProductionModelView   `json:"model"`
	OutputCount      int                    `json:"output_count"`
	Cost             *ProductionCostView    `json:"cost"`
	Error            *ProductionErrorView   `json:"error"`
	Actions          ProductionActions      `json:"actions"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

type ProductionSourceHealthStatus string

const (
	ProductionSourceAvailable   ProductionSourceHealthStatus = "available"
	ProductionSourceUnavailable ProductionSourceHealthStatus = "unavailable"
)

type ProductionSourceHealth struct {
	Source  string                       `json:"source"`
	Status  ProductionSourceHealthStatus `json:"status"`
	Message *string                      `json:"message"`
}

type ProductionRunPage struct {
	ContractVersion string                   `json:"contract_version"`
	ProjectID       contract.ProjectID       `json:"project_id"`
	Items           []ProductionRunSummary   `json:"items"`
	NextCursor      *string                  `json:"next_cursor"`
	SourceHealth    []ProductionSourceHealth `json:"source_health"`
}

type ProductionAssetListItem struct {
	Asset      ProductionAssetView `json:"asset"`
	UsedByRuns []ProductionRunRef  `json:"used_by_runs"`
}

type ProductionAssetPage struct {
	ContractVersion string                    `json:"contract_version"`
	ProjectID       contract.ProjectID        `json:"project_id"`
	Items           []ProductionAssetListItem `json:"items"`
	NextCursor      *string                   `json:"next_cursor"`
	SourceHealth    []ProductionSourceHealth  `json:"source_health"`
}

type ProductionAssetView struct {
	AssetRef         contract.AssetVersionRef `json:"asset_ref"`
	Role             string                   `json:"role"`
	MediaKind        ProductionMediaKind      `json:"media_kind"`
	DisplayName      *string                  `json:"display_name"`
	Availability     string                   `json:"availability"`
	MIMEType         *string                  `json:"mime_type"`
	WidthPixels      *int                     `json:"width_pixels"`
	HeightPixels     *int                     `json:"height_pixels"`
	DurationMS       *int64                   `json:"duration_ms"`
	PreviewURL       *string                  `json:"preview_url"`
	PreviewExpiresAt *time.Time               `json:"preview_expires_at"`
}

type ProductionAttempt struct {
	AttemptCount int               `json:"attempt_count"`
	MaxAttempts  int               `json:"max_attempts"`
	RetryOf      *ProductionRunRef `json:"retry_of"`
}

type ProductionLineage struct {
	SourceTask      *ProductionSourceTask      `json:"source_task"`
	InputAssetRefs  []contract.AssetVersionRef `json:"input_asset_refs"`
	OutputAssetRefs []contract.AssetVersionRef `json:"output_asset_refs"`
}

type ProductionRunEvent struct {
	Ordinal     int       `json:"ordinal"`
	Stage       string    `json:"stage"`
	SafeMessage string    `json:"safe_message"`
	ErrorCode   *string   `json:"error_code"`
	OccurredAt  time.Time `json:"occurred_at"`
}

type ProductionRunDetail struct {
	ContractVersion string                   `json:"contract_version"`
	Summary         ProductionRunSummary     `json:"summary"`
	InputAssets     []ProductionAssetView    `json:"input_assets"`
	OutputAssets    []ProductionAssetView    `json:"output_assets"`
	Parameters      map[string]any           `json:"parameters"`
	PromptRef       *contract.ResourceRef    `json:"prompt_ref"`
	Attempt         ProductionAttempt        `json:"attempt"`
	RetryChain      []ProductionRunRef       `json:"retry_chain"`
	RunEvents       []ProductionRunEvent     `json:"run_events"`
	Lineage         ProductionLineage        `json:"lineage"`
	SourceHealth    []ProductionSourceHealth `json:"source_health"`
}

type ListProductionRunsRequest struct {
	MediaKind     ProductionMediaKind
	Statuses      []ProductionStatus
	SourceTaskID  string
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	Query         string
	Cursor        string
	Limit         int
}

type ListProductionAssetsRequest struct {
	Role      string
	MediaKind ProductionMediaKind
	RunSource ProductionRunSourceKind
	Cursor    string
	Limit     int
}

type ProductionSourceScope struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	ListProductionRunsRequest
	BeforeCreated *time.Time
	BeforeKey     string
}

type ProductionSourceKey struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	ID             string
}

type ProductionSourceRun struct {
	OwnerSystem     string
	Summary         ProductionRunSummary
	InputAssetRefs  []contract.AssetVersionRef
	OutputAssetRefs []contract.AssetVersionRef
	Parameters      map[string]any
	PromptRef       *contract.ResourceRef
	Attempt         ProductionAttempt
	RetryChain      []ProductionRunRef
	RunEvents       []ProductionRunEvent
}

type ProductionSourcePage struct {
	Items   []ProductionSourceRun
	HasMore bool
}

type ProductionRunSource interface {
	Source() ProductionRunSourceKind
	List(context.Context, ProductionSourceScope) (ProductionSourcePage, error)
	Get(context.Context, ProductionSourceKey) (ProductionSourceRun, error)
}

type ProductionAssetReader interface {
	Resolve(context.Context, contract.ActorContext, contract.ProjectID, []contract.AssetVersionRef) ([]ProductionAssetView, error)
}

type ProductionCenterQuery interface {
	ListRuns(context.Context, contract.ActorContext, contract.ProjectID, ListProductionRunsRequest) (ProductionRunPage, error)
	GetRun(context.Context, contract.ActorContext, contract.ProjectID, ProductionRunRef) (ProductionRunDetail, error)
}

type ProductionAssetQuery interface {
	ListAssets(context.Context, contract.ActorContext, contract.ProjectID, ListProductionAssetsRequest) (ProductionAssetPage, error)
}

type ProductionCenterService struct {
	Projects      ActiveProjectResolver
	Sources       []ProductionRunSource
	Assets        ProductionAssetReader
	RetryAdapters []ProductionRetryAdapter
}

type productionCursor struct {
	Version            int               `json:"v"`
	WatermarkCreatedAt time.Time         `json:"watermark_created_at"`
	WatermarkKey       string            `json:"watermark_key"`
	SourceCursors      map[string]string `json:"source_cursors"`
}

type productionAssetCursor struct {
	Version int    `json:"v"`
	Key     string `json:"key"`
}

func (s ProductionCenterService) ListRuns(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request ListProductionRunsRequest) (ProductionRunPage, error) {
	if s.Projects == nil {
		return ProductionRunPage{}, fmt.Errorf("production center project resolver is required")
	}
	if !actor.HasScope(ScopeRead) {
		return ProductionRunPage{}, fmt.Errorf("%s scope is required", ScopeRead)
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ProductionRunPage{}, err
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 100 {
		return ProductionRunPage{}, fmt.Errorf("production limit must be between 1 and 100")
	}
	scope := ProductionSourceScope{OrganizationID: actor.OrganizationID, ProjectID: projectID, ListProductionRunsRequest: request}
	if request.Cursor != "" {
		cursor, err := decodeProductionCursor(request.Cursor)
		if err != nil {
			return ProductionRunPage{}, err
		}
		scope.BeforeCreated = &cursor.WatermarkCreatedAt
		scope.BeforeKey = cursor.WatermarkKey
	}
	all := make([]ProductionSourceRun, 0, request.Limit*len(s.Sources))
	health := make([]ProductionSourceHealth, 0, 4)
	available := 0
	hasMore := false
	registered := make(map[ProductionRunSourceKind]bool, len(s.Sources))
	for _, source := range s.Sources {
		if source == nil {
			continue
		}
		kind := source.Source()
		registered[kind] = true
		page, err := source.List(ctx, scope)
		if err != nil {
			message := "source is temporarily unavailable"
			health = append(health, ProductionSourceHealth{Source: string(kind), Status: ProductionSourceUnavailable, Message: &message})
			continue
		}
		available++
		health = append(health, ProductionSourceHealth{Source: string(kind), Status: ProductionSourceAvailable})
		all = append(all, page.Items...)
		hasMore = hasMore || page.HasMore
	}
	for _, kind := range []ProductionRunSourceKind{ProductionSourceProvider, ProductionSourceCreativeRender, ProductionSourceEditingRender, ProductionSourceAudioRender} {
		if !registered[kind] {
			message := "source is not enabled"
			health = append(health, ProductionSourceHealth{Source: string(kind), Status: ProductionSourceUnavailable, Message: &message})
		}
	}
	if available == 0 {
		return ProductionRunPage{}, ErrProductionSourcesUnavailable
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Summary.CreatedAt.Equal(all[j].Summary.CreatedAt) {
			return all[i].Summary.Ref.Key() > all[j].Summary.Ref.Key()
		}
		return all[i].Summary.CreatedAt.After(all[j].Summary.CreatedAt)
	})
	all = filterProductionRuns(all, request)
	if len(all) > request.Limit {
		hasMore = true
		all = all[:request.Limit]
	}
	items := make([]ProductionRunSummary, 0, len(all))
	for _, run := range all {
		run.Summary.Actions.Retry = actor.HasScope(ScopeWrite) && productionStatusAllowsRetry(run.Summary.NormalizedStatus) && selectProductionRetryAdapter(s.RetryAdapters, run) != nil
		items = append(items, run.Summary)
	}
	var next *string
	if hasMore && len(items) > 0 {
		encoded, err := encodeProductionCursor(items[len(items)-1])
		if err != nil {
			return ProductionRunPage{}, err
		}
		next = &encoded
	}
	return ProductionRunPage{ContractVersion: productionRunPageContractVersion, ProjectID: projectID, Items: items, NextCursor: next, SourceHealth: health}, nil
}

func (s ProductionCenterService) GetRun(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, ref ProductionRunRef) (ProductionRunDetail, error) {
	if s.Projects == nil || !actor.HasScope(ScopeRead) {
		return ProductionRunDetail{}, fmt.Errorf("production center read access is required")
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ProductionRunDetail{}, err
	}
	var selected ProductionRunSource
	for _, source := range s.Sources {
		if source != nil && source.Source() == ref.Source {
			selected = source
			break
		}
	}
	if selected == nil {
		return ProductionRunDetail{}, ErrProductionRunNotFound
	}
	run, err := selected.Get(ctx, ProductionSourceKey{OrganizationID: actor.OrganizationID, ProjectID: projectID, ID: ref.ID})
	if err != nil {
		return ProductionRunDetail{}, err
	}
	run.Summary.Actions.Retry = actor.HasScope(ScopeWrite) && productionStatusAllowsRetry(run.Summary.NormalizedStatus) && selectProductionRetryAdapter(s.RetryAdapters, run) != nil
	inputAssets, outputAssets := []ProductionAssetView{}, []ProductionAssetView{}
	health := []ProductionSourceHealth{{Source: string(ref.Source), Status: ProductionSourceAvailable}}
	if s.Assets != nil {
		inputAssets, err = s.Assets.Resolve(ctx, actor, projectID, run.InputAssetRefs)
		if err == nil {
			for i := range inputAssets {
				inputAssets[i].Role = "input"
			}
			outputAssets, err = s.Assets.Resolve(ctx, actor, projectID, run.OutputAssetRefs)
		}
		if err != nil {
			message := "asset metadata is temporarily unavailable"
			health = append(health, ProductionSourceHealth{Source: "assets", Status: ProductionSourceUnavailable, Message: &message})
			inputAssets, outputAssets = []ProductionAssetView{}, []ProductionAssetView{}
		} else {
			for i := range outputAssets {
				outputAssets[i].Role = "output"
			}
			health = append(health, ProductionSourceHealth{Source: "assets", Status: ProductionSourceAvailable})
		}
	} else {
		message := "asset metadata source is not enabled"
		health = append(health, ProductionSourceHealth{Source: "assets", Status: ProductionSourceUnavailable, Message: &message})
	}
	return ProductionRunDetail{
		ContractVersion: productionRunDetailContractVersion, Summary: run.Summary,
		InputAssets: inputAssets, OutputAssets: outputAssets, Parameters: nonNilParameters(run.Parameters), PromptRef: run.PromptRef,
		Attempt: run.Attempt, RetryChain: nonNilRunRefs(run.RetryChain), RunEvents: nonNilRunEvents(run.RunEvents),
		Lineage:      ProductionLineage{SourceTask: run.Summary.SourceTask, InputAssetRefs: nonNilAssetRefs(run.InputAssetRefs), OutputAssetRefs: nonNilAssetRefs(run.OutputAssetRefs)},
		SourceHealth: health,
	}, nil
}

func nonNilRunEvents(events []ProductionRunEvent) []ProductionRunEvent {
	if events == nil {
		return []ProductionRunEvent{}
	}
	return events
}

func (s ProductionCenterService) ListAssets(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, request ListProductionAssetsRequest) (ProductionAssetPage, error) {
	if s.Projects == nil || !actor.HasScope(ScopeRead) {
		return ProductionAssetPage{}, fmt.Errorf("production center read access is required")
	}
	if _, err := s.Projects.RequireActiveContext(ctx, actor, projectID); err != nil {
		return ProductionAssetPage{}, err
	}
	if request.Limit == 0 {
		request.Limit = 50
	}
	if request.Limit < 1 || request.Limit > 100 {
		return ProductionAssetPage{}, fmt.Errorf("production limit must be between 1 and 100")
	}
	if s.Assets == nil {
		return ProductionAssetPage{}, ErrProductionSourcesUnavailable
	}

	runs := make([]ProductionSourceRun, 0)
	health := make([]ProductionSourceHealth, 0, len(s.Sources)+1)
	available := 0
	registered := make(map[ProductionRunSourceKind]bool, len(s.Sources))
	for _, source := range s.Sources {
		if source == nil || request.RunSource != "" && source.Source() != request.RunSource {
			continue
		}
		kind := source.Source()
		registered[kind] = true
		scope := ProductionSourceScope{OrganizationID: actor.OrganizationID, ProjectID: projectID, ListProductionRunsRequest: ListProductionRunsRequest{Limit: 100}}
		failed := false
		for {
			page, err := source.List(ctx, scope)
			if err != nil {
				message := "source is temporarily unavailable"
				health = append(health, ProductionSourceHealth{Source: string(kind), Status: ProductionSourceUnavailable, Message: &message})
				failed = true
				break
			}
			runs = append(runs, page.Items...)
			if !page.HasMore || len(page.Items) == 0 {
				break
			}
			last := page.Items[len(page.Items)-1].Summary
			scope.BeforeCreated, scope.BeforeKey = &last.CreatedAt, last.Ref.Key()
		}
		if !failed {
			available++
			health = append(health, ProductionSourceHealth{Source: string(kind), Status: ProductionSourceAvailable})
		}
	}
	if request.RunSource == "" {
		for _, kind := range []ProductionRunSourceKind{ProductionSourceProvider, ProductionSourceCreativeRender, ProductionSourceEditingRender, ProductionSourceAudioRender} {
			if !registered[kind] {
				message := "source is not enabled"
				health = append(health, ProductionSourceHealth{Source: string(kind), Status: ProductionSourceUnavailable, Message: &message})
			}
		}
	}
	if available == 0 {
		return ProductionAssetPage{}, ErrProductionSourcesUnavailable
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if runs[i].Summary.CreatedAt.Equal(runs[j].Summary.CreatedAt) {
			return runs[i].Summary.Ref.Key() > runs[j].Summary.Ref.Key()
		}
		return runs[i].Summary.CreatedAt.After(runs[j].Summary.CreatedAt)
	})

	type lineage struct {
		ref     contract.AssetVersionRef
		role    string
		runRefs []ProductionRunRef
	}
	byRef := make(map[contract.AssetVersionRef]*lineage)
	order := make([]contract.AssetVersionRef, 0)
	appendRefs := func(role string, refs []contract.AssetVersionRef, runRef ProductionRunRef) {
		if request.Role != "" && request.Role != role {
			return
		}
		for _, ref := range refs {
			item := byRef[ref]
			if item == nil {
				item = &lineage{ref: ref, role: role}
				byRef[ref] = item
				order = append(order, ref)
			} else if role == "output" {
				item.role = "output"
			}
			if len(item.runRefs) == 0 || item.runRefs[len(item.runRefs)-1] != runRef {
				item.runRefs = append(item.runRefs, runRef)
			}
		}
	}
	for _, run := range runs {
		appendRefs("input", run.InputAssetRefs, run.Summary.Ref)
		appendRefs("output", run.OutputAssetRefs, run.Summary.Ref)
	}

	refs := make([]contract.AssetVersionRef, 0, len(order))
	for _, ref := range order {
		refs = append(refs, ref)
	}
	views, err := s.Assets.Resolve(ctx, actor, projectID, refs)
	if err != nil {
		return ProductionAssetPage{}, ErrProductionSourcesUnavailable
	}
	health = append(health, ProductionSourceHealth{Source: "assets", Status: ProductionSourceAvailable})
	items := make([]ProductionAssetListItem, 0, len(views))
	for _, view := range views {
		entry := byRef[view.AssetRef]
		if entry == nil || request.MediaKind != "" && view.MediaKind != request.MediaKind {
			continue
		}
		view.Role = entry.role
		items = append(items, ProductionAssetListItem{Asset: view, UsedByRuns: nonNilRunRefs(entry.runRefs)})
	}
	sort.SliceStable(items, func(i, j int) bool { return productionAssetKey(items[i]) < productionAssetKey(items[j]) })
	if request.Cursor != "" {
		cursor, err := decodeProductionAssetCursor(request.Cursor)
		if err != nil {
			return ProductionAssetPage{}, err
		}
		filtered := items[:0]
		for _, item := range items {
			if productionAssetKey(item) > cursor.Key {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	var next *string
	if len(items) > request.Limit {
		items = items[:request.Limit]
		value, err := encodeProductionAssetCursor(productionAssetKey(items[len(items)-1]))
		if err != nil {
			return ProductionAssetPage{}, err
		}
		next = &value
	}
	return ProductionAssetPage{ContractVersion: productionAssetPageContractVersion, ProjectID: projectID, Items: items, NextCursor: next, SourceHealth: health}, nil
}

func filterProductionRuns(items []ProductionSourceRun, request ListProductionRunsRequest) []ProductionSourceRun {
	filtered := items[:0]
	statuses := make(map[ProductionStatus]bool, len(request.Statuses))
	for _, status := range request.Statuses {
		statuses[status] = true
	}
	query := strings.ToLower(strings.TrimSpace(request.Query))
	for _, item := range items {
		summary := item.Summary
		if request.MediaKind != "" && summary.MediaKind != request.MediaKind || len(statuses) > 0 && !statuses[summary.NormalizedStatus] {
			continue
		}
		if request.SourceTaskID != "" && (summary.SourceTask == nil || summary.SourceTask.ObjectID != request.SourceTaskID) {
			continue
		}
		if request.CreatedAfter != nil && summary.CreatedAt.Before(*request.CreatedAfter) || request.CreatedBefore != nil && !summary.CreatedAt.Before(*request.CreatedBefore) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(summary.Ref.ID)
			if summary.SourceTask != nil && summary.SourceTask.DisplayName != nil {
				haystack += " " + strings.ToLower(*summary.SourceTask.DisplayName)
			}
			if summary.Error != nil {
				haystack += " " + strings.ToLower(summary.Error.Code)
			}
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func encodeProductionCursor(summary ProductionRunSummary) (string, error) {
	payload, err := json.Marshal(productionCursor{Version: 1, WatermarkCreatedAt: summary.CreatedAt.UTC(), WatermarkKey: summary.Ref.Key(), SourceCursors: map[string]string{}})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeProductionCursor(value string) (productionCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return productionCursor{}, ErrProductionCursorInvalid
	}
	var cursor productionCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != 1 || cursor.WatermarkCreatedAt.IsZero() || strings.TrimSpace(cursor.WatermarkKey) == "" {
		return productionCursor{}, ErrProductionCursorInvalid
	}
	return cursor, nil
}

func productionAssetKey(item ProductionAssetListItem) string {
	return fmt.Sprintf("%s:%010d", item.Asset.AssetRef.AssetID, item.Asset.AssetRef.Version)
}

func encodeProductionAssetCursor(key string) (string, error) {
	payload, err := json.Marshal(productionAssetCursor{Version: 1, Key: key})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeProductionAssetCursor(value string) (productionAssetCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return productionAssetCursor{}, ErrProductionCursorInvalid
	}
	var cursor productionAssetCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != 1 || strings.TrimSpace(cursor.Key) == "" {
		return productionAssetCursor{}, ErrProductionCursorInvalid
	}
	return cursor, nil
}

func nonNilParameters(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func nonNilAssetRefs(value []contract.AssetVersionRef) []contract.AssetVersionRef {
	if value == nil {
		return []contract.AssetVersionRef{}
	}
	return value
}

func nonNilRunRefs(value []ProductionRunRef) []ProductionRunRef {
	if value == nil {
		return []ProductionRunRef{}
	}
	return value
}

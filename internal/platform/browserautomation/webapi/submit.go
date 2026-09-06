package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/integrations/oceanengine"
	"github.com/shikanon/cookies/internal/platform/browserautomation"
)

// CompiledObject describes the next pending staged create. The delivery-side
// payload source derives it from the immutable plan version and the pending
// platform entity mappings; the account-calibrated reference IDs let the
// adapter verify that the plan targets the calibrated account objects.
type CompiledObject struct {
	Kind       string // "project" | "promotion"
	InternalID string
	Name       string
	// StartUnix and EndUnix carry the schedule as UTC epoch seconds. Zero keeps
	// the calibrated template value.
	StartUnix int64
	EndUnix   int64
	// BidYuan carries the plan bid in yuan. Nil keeps the template value.
	BidYuan *float64
	// ExternalAction is the selected optimization target ID from the immutable
	// plan. Project creation must not inherit this value from a local template.
	ExternalAction string
	// ProductReferenceID is the plan's resolved product reference. A non-empty
	// value must equal the template product_id or the adapter stops before the
	// write.
	ProductReferenceID string
	// MaterialReferenceIDs are the plan's resolved base material references. A
	// non-empty list must match the template material video IDs.
	MaterialReferenceIDs []string
	// DependsOnPlatformID is the confirmed project platform ID for a promotion.
	DependsOnPlatformID string
}

// PayloadSource produces the next pending staged create for a run. It returns
// pending=false when every staged mapping for the run is already confirmed.
type PayloadSource interface {
	CompileNext(ctx context.Context, run browserautomation.BrowserRpaRun) (CompiledObject, bool, error)
}

// CreateTemplates carries the account-calibrated request payloads captured
// from the dedicated test account. The values identify calibrated platform
// references (product, landing asset, materials) and contain no credentials.
// They live in a git-ignored local file loaded through TemplateSource.
type CreateTemplates struct {
	Project   map[string]any `json:"project_create"`
	Promotion map[string]any `json:"promotion_create"`
}

// TemplateSource loads the calibrated create templates.
type TemplateSource interface {
	Load(ctx context.Context) (CreateTemplates, error)
}

// WriteSession bundles one decrypted write client with its read client.
type WriteSession struct {
	Writer *oceanengine.WriteClient
	Reader *oceanengine.Client
	Close  func()
}

// WriteSessionFactory opens one decrypted Connector session for a run.
type WriteSessionFactory interface {
	OpenSession(ctx context.Context, run browserautomation.BrowserRpaRun) (WriteSession, error)
}

var (
	// ErrTemplateNotConfigured reports that the calibrated template file is
	// absent. The driver stays fail-closed without it.
	ErrTemplateNotConfigured = errors.New("Ocean Engine Web API calibrated template is not configured")
	// ErrTemplateMismatch reports that the plan references platform objects the
	// calibrated template does not cover.
	ErrTemplateMismatch = errors.New("Ocean Engine Web API plan references are outside the calibrated template")
	// ErrPendingObjectMissing reports a submit without a pending staged object.
	ErrPendingObjectMissing = errors.New("Ocean Engine Web API run has no pending staged object")
	// ErrPlatformIdentityMissing reports a 2xx create response without a
	// platform object ID.
	ErrPlatformIdentityMissing = errors.New("Ocean Engine Web API create response has no platform object ID")
)

// Submit executes the next pending staged create with the confirmed Web API
// contract: name precheck, one protected POST, by-ID reconciliation, and field
// verification. It never retries a write and leaves mapping confirmation to
// the worker's staged-create loop.
func (a Adapter) Submit(ctx context.Context, run browserautomation.BrowserRpaRun, attempt browserautomation.ControlledActionAttempt, confirmToken string) (browserautomation.WorkerOutcome, browserautomation.PreparedPage, error) {
	if err := a.CheckSubmit(run); err != nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, err
	}
	if a.PayloadSource == nil || a.Templates == nil || a.SessionFactory == nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, browserautomation.ErrEnvironmentUnavailable
	}
	object, pending, err := a.PayloadSource.CompileNext(ctx, run)
	if err != nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, err
	}
	if !pending || object.Kind != "project" && object.Kind != "promotion" || object.Name == "" || object.InternalID == "" {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, fmt.Errorf("%w: next staged object is incomplete", browserautomation.ErrInvalidContract)
	}
	templates, err := a.Templates.Load(ctx)
	if err != nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, err
	}
	payload, err := assembleCreatePayload(object, templates)
	if err != nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, err
	}
	session, err := a.SessionFactory.OpenSession(ctx, run)
	if err != nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, err
	}
	defer session.Close()

	path := oceanengine.ProjectCreatePath
	if object.Kind == "promotion" {
		path = oceanengine.PromotionCreatePath
	}
	if err := precheckName(ctx, session.Reader, object); err != nil {
		return browserautomation.WorkerFailed, browserautomation.PreparedPage{}, err
	}
	response, writeErr := session.Writer.SubmitJSON(ctx, path, payload)
	responseObject := decodeResponseObject(response.Body)
	businessCode := responseBusinessCode(responseObject)
	if writeErr != nil && businessCode == 0 {
		// Transport-level uncertainty: the effect is unproven. Report unknown
		// without a second write; the worker's reconciliation loop resolves it.
		return browserautomation.WorkerResultUnknown, stagedPage(object, "", "not_checked"), nil
	}
	platformID := firstNumericID(responseObject, object.Kind)
	if businessCode != 0 {
		return browserautomation.WorkerFailed, stagedPage(object, "", "not_checked"), fmt.Errorf("%w: platform business code %d", browserautomation.ErrInvalidContract, businessCode)
	}
	if platformID == "" {
		return browserautomation.WorkerResultUnknown, stagedPage(object, "", "not_checked"), fmt.Errorf("%w: %s create", ErrPlatformIdentityMissing, object.Kind)
	}
	fieldStatus, readback, reconcileErr := reconcileCreatedObject(ctx, session.Reader, object, platformID, payload)
	if reconcileErr != nil {
		return browserautomation.WorkerResultUnknown, stagedPage(object, platformID, "not_checked"), nil
	}
	if fieldStatus != "matched" {
		return browserautomation.WorkerPartial, stagedPageWithReadback(object, platformID, fieldStatus, readback), nil
	}
	return browserautomation.WorkerSuccess, stagedPageWithReadback(object, platformID, fieldStatus, readback), nil
}

// assembleCreatePayload clones the calibrated template and injects the plan
// identity fields. Reference mismatches stop before any write.
func assembleCreatePayload(object CompiledObject, templates CreateTemplates) (map[string]any, error) {
	var template map[string]any
	switch object.Kind {
	case "project":
		template = templates.Project
	case "promotion":
		template = templates.Promotion
	}
	if len(template) == 0 {
		return nil, fmt.Errorf("%w: %s create template is absent", ErrTemplateNotConfigured, object.Kind)
	}
	payload := maps.Clone(template)
	payload["name"] = object.Name
	switch object.Kind {
	case "project":
		if object.StartUnix > 0 {
			payload["start_time"] = strconv.FormatInt(object.StartUnix, 10)
		}
		if object.EndUnix > 0 {
			payload["end_time"] = strconv.FormatInt(object.EndUnix, 10)
		}
		if object.BidYuan != nil {
			payload["bid"] = *object.BidYuan
		}
		if object.ExternalAction == "" {
			return nil, fmt.Errorf("%w: project external_action is absent", browserautomation.ErrInvalidContract)
		}
		payload["external_action"] = object.ExternalAction
	case "promotion":
		if object.DependsOnPlatformID == "" {
			return nil, fmt.Errorf("%w: promotion without a confirmed project binding", browserautomation.ErrInvalidContract)
		}
		payload["project_id"] = object.DependsOnPlatformID
		payload["check_hash"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	}
	if err := validateCalibratedReferences(object, template); err != nil {
		return nil, err
	}
	return payload, nil
}

// validateCalibratedReferences stops when the plan references platform objects
// the calibrated template does not cover. It never compares names.
func validateCalibratedReferences(object CompiledObject, template map[string]any) error {
	if object.ProductReferenceID != "" {
		if templateProduct := firstNumericID(template, "project"); templateProduct != "" && templateProduct != object.ProductReferenceID {
			return fmt.Errorf("%w: plan product %s is not the calibrated product %s", ErrTemplateMismatch, object.ProductReferenceID, templateProduct)
		}
	}
	if len(object.MaterialReferenceIDs) == 0 {
		return nil
	}
	templateMaterials := templateMaterialIDs(template)
	if len(templateMaterials) == 0 {
		return nil
	}
	for _, reference := range object.MaterialReferenceIDs {
		if reference == "" {
			continue
		}
		if !containsString(templateMaterials, reference) {
			return fmt.Errorf("%w: plan material %s is outside the calibrated template", ErrTemplateMismatch, reference)
		}
	}
	return nil
}

func templateMaterialIDs(template map[string]any) []string {
	group, _ := template["material_group"].(map[string]any)
	if group == nil {
		return nil
	}
	videos, _ := group["video_material_info"].([]any)
	ids := make([]string, 0, len(videos))
	for _, item := range videos {
		entry, _ := item.(map[string]any)
		info, _ := entry["video_info"].(map[string]any)
		if info == nil {
			continue
		}
		if id, ok := info["video_id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// precheckName mirrors the browser order: the name availability query runs
// immediately before the create.
func precheckName(ctx context.Context, reader *oceanengine.Client, object CompiledObject) error {
	if reader == nil {
		return nil
	}
	if object.Kind == "project" {
		_, err := reader.CheckProjectName(ctx, object.Name)
		return err
	}
	_, err := reader.CheckPromotionName(ctx, object.Name)
	return err
}

// reconcileCreatedObject verifies the created object through the same by-ID
// read the browser uses after a create. The name must match exactly and the
// platform ID must agree with the read-back row.
func reconcileCreatedObject(ctx context.Context, reader *oceanengine.Client, object CompiledObject, platformID string, payload map[string]any) (string, map[string]string, error) {
	if reader == nil || platformID == "" {
		return "not_checked", nil, fmt.Errorf("reconciliation read is unavailable")
	}
	var row map[string]any
	var err error
	if object.Kind == "project" {
		row, err = projectRowByID(ctx, reader, platformID)
	} else {
		row, err = promotionRowByID(ctx, reader, platformID)
	}
	if err != nil {
		return "not_checked", nil, err
	}
	readback := map[string]string{
		"platform_object_id": platformID,
		"reconciliation":     "matched",
	}
	if object.Kind == "project" && object.ExternalAction != "" {
		readback["expected_external_action"] = object.ExternalAction
		if echoed := findScalarString(row, "external_action"); echoed != "" {
			readback["external_action"] = echoed
		}
	}
	nameKey := "project_name"
	if object.Kind == "promotion" {
		nameKey = "promotion_name"
	}
	name, _ := row[nameKey].(string)
	if object.Kind == "project" && name == "" {
		name, _ = row["name"].(string)
	}
	if name != object.Name {
		return "mismatched", readback, nil
	}
	if status := reconcileScheduleAndBid(object, payload, row); status != "matched" {
		readback["field_reconciliation_status"] = status
		return status, readback, nil
	}
	readback["field_reconciliation_status"] = "matched"
	return "matched", readback, nil
}

func projectRowByID(ctx context.Context, reader *oceanengine.Client, platformID string) (map[string]any, error) {
	value, err := reader.ProjectDetails(ctx, platformID)
	if err != nil {
		return nil, err
	}
	rows := oceanengine.FlattenNamedRows(value, "project_name")
	if len(rows) == 0 {
		rows = oceanengine.FlattenNamedRows(value, "name")
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("project %s detail returned %d rows", platformID, len(rows))
	}
	return rows[0], nil
}

func promotionRowByID(ctx context.Context, reader *oceanengine.Client, platformID string) (map[string]any, error) {
	value, err := reader.PromotionMaterials(ctx, platformID, false)
	if err != nil {
		return nil, err
	}
	rows := oceanengine.FlattenNamedRows(value, "promotion_name")
	if len(rows) != 1 {
		return nil, fmt.Errorf("promotion %s read returned %d rows", platformID, len(rows))
	}
	return rows[0], nil
}

// reconcileScheduleAndBid compares the plan fields the platform echoes back.
// Absent fields stay unchecked and keep the status matched.
func reconcileScheduleAndBid(object CompiledObject, payload map[string]any, row map[string]any) string {
	if object.Kind == "project" && object.ExternalAction != "" {
		echoed := findScalarString(row, "external_action")
		if echoed == "" {
			return "not_checked"
		}
		if echoed != object.ExternalAction {
			return "mismatched"
		}
	}
	if object.StartUnix > 0 {
		if echoed, ok := row["start_time"].(string); ok && echoed != "" {
			if echoed != formatPlatformTime(object.StartUnix) {
				return "mismatched"
			}
		}
	}
	if object.EndUnix > 0 {
		if echoed, ok := row["end_time"].(string); ok && echoed != "" {
			if echoed != formatPlatformTime(object.EndUnix) {
				return "mismatched"
			}
		}
	}
	if bid, ok := payload["bid"].(float64); ok {
		if echoed := firstDecimalString(row["project_bid"], row["ad_bid"]); echoed != "" {
			if !decimalEquals(echoed, bid) {
				return "mismatched"
			}
		}
	}
	return "matched"
}

func firstScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func findScalarString(value any, key string) string {
	switch typed := value.(type) {
	case map[string]any:
		if result := firstScalarString(typed[key]); result != "" {
			return result
		}
		for _, child := range typed {
			if result := findScalarString(child, key); result != "" {
				return result
			}
		}
	case []any:
		for _, child := range typed {
			if result := findScalarString(child, key); result != "" {
				return result
			}
		}
	}
	return ""
}

func formatPlatformTime(unix int64) string {
	return time.Unix(unix, 0).In(time.FixedZone("UTC8", 8*60*60)).Format("2006-01-02 15:04:05")
}

func firstDecimalString(values ...any) string {
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			return text
		}
	}
	return ""
}

func decimalEquals(echoed string, bid float64) bool {
	parsed, err := strconv.ParseFloat(echoed, 64)
	if err != nil {
		return false
	}
	// Money echoes carry two decimals; compare at cent precision.
	return strconv.FormatInt(int64(parsed*100+copysignEpsilon(parsed)), 10) == strconv.FormatInt(int64(bid*100+copysignEpsilon(bid)), 10)
}

func copysignEpsilon(value float64) float64 {
	if value < 0 {
		return -0.5
	}
	return 0.5
}

func stagedPage(object CompiledObject, platformID, fieldStatus string) browserautomation.PreparedPage {
	return stagedPageWithReadback(object, platformID, fieldStatus, nil)
}

func stagedPageWithReadback(object CompiledObject, platformID, fieldStatus string, readback map[string]string) browserautomation.PreparedPage {
	page := browserautomation.PreparedPage{
		BeforeFacts: map[string]string{
			"execution_driver": string(browserautomation.ExecutionDriverOceanEngineWebAPI),
			"contract_version": ContractVersion,
			"account_match":    "true",
		},
		Readback: map[string]string{
			"write_gate": "ready",
		},
		DiffKeys:           []string{},
		PageRef:            "oceanengine-web-api://submit/" + object.Kind + "/" + object.InternalID,
		InternalObjectKind: object.Kind,
		InternalObjectID:   object.InternalID,
		SelectorVersion:    SelectorVersion,
		ActionVersion:      ActionVersion,
	}
	if platformID != "" {
		page.Readback["platform_object_id"] = platformID
		page.Readback["reconciliation"] = map[bool]string{true: "matched", false: "unmatched"}[fieldStatus == "matched"]
		page.Readback["field_reconciliation_status"] = fieldStatus
	}
	for key, value := range readback {
		page.Readback[key] = value
	}
	return page
}

func decodeResponseObject(data json.RawMessage) map[string]any {
	var value map[string]any
	_ = json.Unmarshal(data, &value)
	return value
}

func responseBusinessCode(value map[string]any) int {
	switch code := value["code"].(type) {
	case float64:
		return int(code)
	case json.Number:
		parsed, _ := code.Int64()
		return int(parsed)
	}
	return 0
}

func firstNumericID(value any, kind string) string {
	preferred := []string{"project_id", "campaign_id", "id"}
	if kind == "promotion" {
		preferred = []string{"promotion_id", "id"}
	}
	return findNumericIDWithKeys(value, preferred...)
}

func findNumericIDWithKeys(value any, preferred ...string) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range preferred {
			if found := numericString(typed[key]); found != "" {
				return found
			}
		}
		for _, child := range typed {
			if found := findNumericIDWithKeys(child, preferred...); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range typed {
			if found := findNumericIDWithKeys(child, preferred...); found != "" {
				return found
			}
		}
	}
	return ""
}

func numericString(value any) string {
	switch typed := value.(type) {
	case string:
		if typed != "" && !containsNonDigit(typed) {
			return typed
		}
	case float64:
		if typed > 0 && typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case json.Number:
		return string(typed)
	}
	return ""
}

func containsNonDigit(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return true
		}
	}
	return false
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

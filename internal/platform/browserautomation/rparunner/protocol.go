package rparunner

import "encoding/json"

// Wire protocol between the Go control plane and the Playwright RPA runner
// subprocess. The plan is written to the subprocess stdin; the subprocess
// answers with exactly one JSON result document on stdout. Exit codes only
// express infrastructure failure, never business outcomes.

const (
	PlanSchemaV2   = "oceanengine-playwright-rpa/v2"
	ResultSchemaV1 = "oceanengine-playwright-rpa-result/v1"
	PlanSchemaV3   = "oceanengine-playwright-rpa-plan/v3"
	ResultSchemaV2 = "oceanengine-playwright-rpa-result/v2"

	SelectorVersion = "playwright-rpa-locator/v1"
	ActionVersion   = "playwright-rpa-action/v1"
)

type LocatorSpec struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type RpaField struct {
	Key     string      `json:"key"`
	Value   any         `json:"value"`
	Locator LocatorSpec `json:"locator"`
}

type RpaStep struct {
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	Page     string       `json:"page,omitempty"`
	PageKind string       `json:"page_kind,omitempty"`
	Fields   []RpaField   `json:"fields,omitempty"`
	Locator  *LocatorSpec `json:"locator,omitempty"`
	// ScopeChecks must each resolve to at least one element before the step
	// touches any field; PresenceChecks must each resolve to at least one
	// element and are recorded as readback evidence.
	ScopeChecks    []LocatorSpec `json:"scope_checks,omitempty"`
	PresenceChecks []LocatorSpec `json:"presence_checks,omitempty"`
	// RemoteWrite marks the step as crossing the final write boundary. The
	// runner executes it only when the plan allows remote writes.
	RemoteWrite bool `json:"remoteWrite,omitempty"`
}

type RpaPlan struct {
	SchemaVersion    string    `json:"schemaVersion"`
	Browser          string    `json:"browser"`
	Mode             string    `json:"mode"`
	AccountID        string    `json:"accountId"`
	Steps            []RpaStep `json:"steps"`
	AllowRemoteWrite bool      `json:"allowRemoteWrite"`
	EvidenceRoot     string    `json:"evidenceRoot,omitempty"`
	RunID            string    `json:"runId,omitempty"`
	// Policy-derived guards verified by the runner against the live page.
	AllowedProtocols  []string `json:"allowedProtocols,omitempty"`
	AllowedHosts      []string `json:"allowedHosts,omitempty"`
	ExpectedObjectID  string   `json:"expectedObjectId,omitempty"`
	ObjectIDQueryKey  string   `json:"objectIdQueryKey,omitempty"`
	AccountIDQueryKey string   `json:"accountIdQueryKey,omitempty"`
}

type StepResult struct {
	ID             string            `json:"id"`
	Status         string            `json:"status"`
	BeforeFacts    map[string]string `json:"before_facts,omitempty"`
	Readback       FlexibleReadback  `json:"readback,omitempty"`
	DiffKeys       []string          `json:"diff_keys,omitempty"`
	PageReference  string            `json:"page_reference,omitempty"`
	ScreenshotPath string            `json:"screenshot_path,omitempty"`
}

// FlexibleReadback accepts both aggregate readback objects and scalar field
// values. Runner v3 emits one scalar for field steps and one object for its
// aggregate readback step.
type FlexibleReadback map[string]any

func (r *FlexibleReadback) UnmarshalJSON(payload []byte) error {
	if string(payload) == "null" {
		*r = nil
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err == nil {
		*r = object
		return nil
	}
	var scalar any
	if err := json.Unmarshal(payload, &scalar); err != nil {
		return err
	}
	*r = FlexibleReadback{"value": scalar}
	return nil
}

type RpaResult struct {
	SchemaVersion       string               `json:"schema_version"`
	Outcome             string               `json:"outcome"`
	ErrorCode           string               `json:"error_code"`
	ErrorMessage        string               `json:"error_message,omitempty"`
	FinalClickPerformed bool                 `json:"final_click_performed"`
	CreatedObjectID     string               `json:"created_object_id,omitempty"`
	Reconciliation      string               `json:"reconciliation,omitempty"`
	FieldReconciliation *FieldReconciliation `json:"field_reconciliation,omitempty"`
	Steps               []StepResult         `json:"steps,omitempty"`
}

type ReconciledField struct {
	FieldKey string `json:"field_key"`
	Expected any    `json:"expected,omitempty"`
	Observed any    `json:"observed,omitempty"`
	Status   string `json:"status"`
}

type FieldReconciliation struct {
	Status string            `json:"status"`
	Fields []ReconciledField `json:"fields"`
}

// Error codes reported by the runner.
const (
	CodeOK                     = "ok"
	CodePageDrift              = "page_drift"
	CodeAccountMismatch        = "account_mismatch"
	CodeLocatorNotUnique       = "locator_not_unique"
	CodeCDPUnavailable         = "cdp_unavailable"
	CodeTimeout                = "timeout"
	CodeWriteBlocked           = "write_blocked"
	CodePlatformRejected       = "platform_rejected"
	CodeEnvironmentUnavailable = "environment_unavailable"
	CodeInternal               = "internal"
)

// Outcomes reported by the runner, mirroring browserautomation.WorkerOutcome.
const (
	OutcomeSuccess          = "success"
	OutcomeSuccessWithDrift = "success_with_drift"
	OutcomeFailed           = "failed"
	OutcomePartial          = "partial"
	OutcomeResultUnknown    = "result_unknown"
)

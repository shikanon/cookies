package webapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
)

const (
	ContractVersion = "oceanengine-web-api-contract/v1"
	SelectorVersion = "oceanengine-web-api/session/v1"
	ActionVersion   = "oceanengine-web-api/action/v1"
)

var (
	ErrWriteDisabled       = errors.New("Ocean Engine Web API write is disabled")
	ErrAccountNotAllowed   = errors.New("Ocean Engine Web API account is not allowed")
	ErrContractNotCaptured = errors.New("Ocean Engine Web API response and reconciliation contract is not captured")
)

type PlanCompiler interface {
	CompilePrepareV3(context.Context, browserautomation.BrowserRpaRun, browserautomation.SitePolicy) (json.RawMessage, error)
}

type SessionChecker interface {
	Check(context.Context, browserautomation.BrowserRpaRun) error
}

type Adapter struct {
	Compiler         PlanCompiler
	Policies         browserautomation.Repository
	Sessions         SessionChecker
	WriteEnabled     bool
	AccountAllowlist []string
	// PayloadSource produces the next pending staged create from the immutable
	// plan version and the pending platform entity mappings.
	PayloadSource PayloadSource
	// Templates loads the account-calibrated create payloads from the local
	// git-ignored template file.
	Templates TemplateSource
	// SessionFactory opens one decrypted Connector session per submit.
	SessionFactory WriteSessionFactory
}

var _ browserautomation.WorkerAdapter = Adapter{}
var _ browserautomation.WorkerPlanAdapter = Adapter{}
var _ browserautomation.WorkerSubmitGate = Adapter{}

func (a Adapter) Plan(ctx context.Context, run browserautomation.BrowserRpaRun) (json.RawMessage, error) {
	if a.Compiler == nil || a.Policies == nil {
		return nil, browserautomation.ErrEnvironmentUnavailable
	}
	policy, err := a.Policies.GetSitePolicy(ctx, run.OrganizationID, run.ProjectID, run.PolicyID)
	if err != nil {
		return nil, err
	}
	return a.Compiler.CompilePrepareV3(ctx, run, policy)
}

func (a Adapter) Prepare(ctx context.Context, run browserautomation.BrowserRpaRun) (browserautomation.PreparedPage, error) {
	if a.Sessions == nil {
		return browserautomation.PreparedPage{}, browserautomation.ErrEnvironmentUnavailable
	}
	if err := a.Sessions.Check(ctx, run); err != nil {
		return browserautomation.PreparedPage{}, err
	}
	plan, err := a.Plan(ctx, run)
	if err != nil {
		return browserautomation.PreparedPage{}, err
	}
	digest := sha256.Sum256(plan)
	return browserautomation.PreparedPage{
		BeforeFacts: map[string]string{
			"execution_driver": string(browserautomation.ExecutionDriverOceanEngineWebAPI),
			"contract_version": ContractVersion,
			"account_match":    "true",
		},
		Readback: map[string]string{
			"compiled_input_sha256": hex.EncodeToString(digest[:]),
			"write_gate":            a.writeGate(run),
		},
		DiffKeys:        []string{},
		PageRef:         "oceanengine-web-api://prepare/" + run.ID,
		SelectorVersion: SelectorVersion,
		ActionVersion:   ActionVersion,
	}, nil
}

// CheckSubmit reports whether this driver may execute a controlled write. The
// write switch, the account allowlist, and the wired contract plumbing
// (payload source, calibrated templates, session factory) must all be ready.
func (a Adapter) CheckSubmit(run browserautomation.BrowserRpaRun) error {
	if !a.WriteEnabled {
		return ErrWriteDisabled
	}
	if !slices.Contains(a.AccountAllowlist, run.AccountID) {
		return ErrAccountNotAllowed
	}
	if a.PayloadSource == nil || a.Templates == nil || a.SessionFactory == nil {
		return ErrContractNotCaptured
	}
	return nil
}

func (a Adapter) writeGate(run browserautomation.BrowserRpaRun) string {
	if err := a.CheckSubmit(run); err != nil {
		return strings.TrimPrefix(fmt.Sprint(err), "Ocean Engine ")
	}
	return "ready"
}

package creative

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
)

const productionRetryContractVersion = "creative-production-retry/v1"

var (
	ErrProductionRetryNotAllowed             = errors.New("production retry is not allowed")
	ErrProductionRetryRequiresSourceWorkflow = errors.New("production retry requires source workflow")
	ErrProductionInputAssetUnavailable       = errors.New("production input asset is unavailable")
	ErrProductionIdempotencyConflict         = errors.New("production idempotency conflict")
)

type ProductionRetryStatus string

const ProductionRetryAccepted ProductionRetryStatus = "accepted"

type ProductionRetryResult struct {
	ContractVersion string                `json:"contract_version"`
	Status          ProductionRetryStatus `json:"status"`
	PreviousRun     ProductionRunRef      `json:"previous_run"`
	NewRun          ProductionRunRef      `json:"new_run"`
	SourceTask      *ProductionSourceTask `json:"source_task"`
}

type ProductionProblem struct {
	ContractVersion string                `json:"contract_version"`
	Code            string                `json:"code"`
	Message         string                `json:"message"`
	Retryable       bool                  `json:"retryable"`
	SourceTask      *ProductionSourceTask `json:"source_task"`
}

type ProductionRetryRequiresSourceWorkflowError struct {
	SourceTask *ProductionSourceTask
}

func (e ProductionRetryRequiresSourceWorkflowError) Error() string {
	return ErrProductionRetryRequiresSourceWorkflow.Error()
}

func (e ProductionRetryRequiresSourceWorkflowError) Unwrap() error {
	return ErrProductionRetryRequiresSourceWorkflow
}

type ProductionRetryCommand interface {
	Retry(context.Context, contract.RequestContext, contract.ProjectID, ProductionRunRef, contract.IdempotencyKey) (ProductionRetryResult, error)
}

// ProductionRetryAdapter is deliberately narrow: Production Center decides
// whether a command may be dispatched, while each owner workflow alone knows
// how to reconstruct immutable input and create the next Attempt/Job.
type ProductionRetryAdapter interface {
	Supports(ProductionSourceRun) bool
	Retry(context.Context, ProductionRetryContext) (ProductionRunRef, error)
}

type ProductionRetryContext struct {
	RequestContext contract.RequestContext
	ProjectID      contract.ProjectID
	Run            ProductionSourceRun
	IdempotencyKey contract.IdempotencyKey
}

type ProductionRetryClaim struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	IdempotencyKey contract.IdempotencyKey
	RequestHash    string
	PreviousRun    ProductionRunRef
	Actor          contract.Principal
	CreatedAt      time.Time
}

// ProductionRetryLedger owns cross-source idempotency. A completed claim
// returns its immutable result; an unfinished same-request claim may safely be
// dispatched again because every registered owner adapter is also idempotent.
type ProductionRetryLedger interface {
	Claim(context.Context, ProductionRetryClaim) (*ProductionRunRef, error)
	Complete(context.Context, ProductionRetryClaim, ProductionRunRef) error
}

type ProductionRetryAuditEvent struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	Actor          contract.Principal
	Action         string
	PreviousRun    ProductionRunRef
	NewRun         ProductionRunRef
	OccurredAt     time.Time
}

type ProductionRetryAuditWriter interface {
	AppendProductionRetryAudit(context.Context, ProductionRetryAuditEvent) error
}

type ProductionRetryService struct {
	Projects ActiveProjectResolver
	Sources  []ProductionRunSource
	Adapters []ProductionRetryAdapter
	Ledger   ProductionRetryLedger
	Audit    ProductionRetryAuditWriter
	Now      func() time.Time
}

func (s ProductionRetryService) Retry(ctx context.Context, rc contract.RequestContext, projectID contract.ProjectID, ref ProductionRunRef, key contract.IdempotencyKey) (ProductionRetryResult, error) {
	if s.Projects == nil || s.Ledger == nil {
		return ProductionRetryResult{}, fmt.Errorf("production retry dependencies are incomplete")
	}
	if err := rc.Validate(); err != nil {
		return ProductionRetryResult{}, err
	}
	if err := key.Validate(); err != nil {
		return ProductionRetryResult{}, err
	}
	if !rc.Actor.HasScope(ScopeWrite) {
		return ProductionRetryResult{}, ErrProductionRetryNotAllowed
	}
	if _, err := s.Projects.RequireActiveContext(ctx, rc.Actor, projectID); err != nil {
		return ProductionRetryResult{}, err
	}
	run, err := findProductionRun(ctx, s.Sources, rc.Actor.OrganizationID, projectID, ref)
	if err != nil {
		return ProductionRetryResult{}, err
	}
	if !productionStatusAllowsRetry(run.Summary.NormalizedStatus) {
		return ProductionRetryResult{}, ErrProductionRetryNotAllowed
	}
	adapter := selectProductionRetryAdapter(s.Adapters, run)
	if adapter == nil {
		return ProductionRetryResult{}, ProductionRetryRequiresSourceWorkflowError{SourceTask: run.Summary.SourceTask}
	}
	hash, err := productionRetryRequestHash(projectID, ref)
	if err != nil {
		return ProductionRetryResult{}, err
	}
	claim := ProductionRetryClaim{
		OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, IdempotencyKey: key,
		RequestHash: hash, PreviousRun: ref, Actor: rc.Actor.Principal, CreatedAt: s.now(),
	}
	completed, err := s.Ledger.Claim(ctx, claim)
	if err != nil {
		return ProductionRetryResult{}, err
	}
	if completed != nil {
		return newProductionRetryResult(ref, *completed, run.Summary.SourceTask), nil
	}
	newRun, err := adapter.Retry(ctx, ProductionRetryContext{RequestContext: rc, ProjectID: projectID, Run: run, IdempotencyKey: key})
	if err != nil {
		return ProductionRetryResult{}, err
	}
	if strings.TrimSpace(newRun.ID) == "" || newRun == ref {
		return ProductionRetryResult{}, fmt.Errorf("production retry adapter returned an invalid new run")
	}
	if err := s.Ledger.Complete(ctx, claim, newRun); err != nil {
		return ProductionRetryResult{}, err
	}
	if s.Audit != nil {
		_ = s.Audit.AppendProductionRetryAudit(ctx, ProductionRetryAuditEvent{
			OrganizationID: rc.Actor.OrganizationID, ProjectID: projectID, Actor: rc.Actor.Principal,
			Action: "creative.production.retry", PreviousRun: ref, NewRun: newRun, OccurredAt: s.now(),
		})
	}
	return newProductionRetryResult(ref, newRun, run.Summary.SourceTask), nil
}

func (s ProductionRetryService) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func findProductionRun(ctx context.Context, sources []ProductionRunSource, organizationID contract.OrganizationID, projectID contract.ProjectID, ref ProductionRunRef) (ProductionSourceRun, error) {
	for _, source := range sources {
		if source != nil && source.Source() == ref.Source {
			return source.Get(ctx, ProductionSourceKey{OrganizationID: organizationID, ProjectID: projectID, ID: ref.ID})
		}
	}
	return ProductionSourceRun{}, ErrProductionRunNotFound
}

func selectProductionRetryAdapter(adapters []ProductionRetryAdapter, run ProductionSourceRun) ProductionRetryAdapter {
	for _, adapter := range adapters {
		if adapter != nil && adapter.Supports(run) {
			return adapter
		}
	}
	return nil
}

func productionStatusAllowsRetry(status ProductionStatus) bool {
	return status == ProductionFailed || status == ProductionExpired || status == ProductionPartiallySucceeded
}

func productionRetryRequestHash(projectID contract.ProjectID, ref ProductionRunRef) (string, error) {
	payload, err := json.Marshal(struct {
		ProjectID contract.ProjectID      `json:"project_id"`
		Source    ProductionRunSourceKind `json:"source"`
		RunID     string                  `json:"run_id"`
	}{ProjectID: projectID, Source: ref.Source, RunID: ref.ID})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func newProductionRetryResult(previous, next ProductionRunRef, sourceTask *ProductionSourceTask) ProductionRetryResult {
	return ProductionRetryResult{ContractVersion: productionRetryContractVersion, Status: ProductionRetryAccepted, PreviousRun: previous, NewRun: next, SourceTask: sourceTask}
}

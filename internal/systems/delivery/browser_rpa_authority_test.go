package delivery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
)

func TestControlledActionsBelongToOtherObjects(t *testing.T) {
	tests := []struct {
		name                                   string
		attempts, confirmations, target, other int
		want                                   bool
	}{
		{name: "project submit does not block untouched promotion", attempts: 1, confirmations: 1, target: 0, other: 1, want: true},
		{name: "target submit blocks recovery", attempts: 1, confirmations: 1, target: 1, other: 0, want: false},
		{name: "unaccounted attempt blocks recovery", attempts: 2, confirmations: 2, target: 0, other: 1, want: false},
		{name: "unconsumed confirmation blocks recovery", attempts: 1, confirmations: 2, target: 0, other: 1, want: false},
		{name: "no actions use the original safe path", attempts: 0, confirmations: 0, target: 0, other: 0, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := controlledActionsBelongToOtherObjects(test.attempts, test.confirmations, test.target, test.other); got != test.want {
				t.Fatalf("got %t want %t", got, test.want)
			}
		})
	}
}

func TestPreparedRunCanRebindOnlyBeforeSubmit(t *testing.T) {
	tests := []struct {
		name                                      string
		attempts, confirmations, noClick, clicked int
		want                                      bool
	}{
		{name: "prepare evidence is safe", noClick: 1, want: true},
		{name: "missing evidence is unsafe", want: false},
		{name: "submit attempt is unsafe", attempts: 1, confirmations: 1, noClick: 1, want: false},
		{name: "click evidence is unsafe", noClick: 1, clicked: 1, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := preparedRunCanRebind(test.attempts, test.confirmations, test.noClick, test.clicked); got != test.want {
				t.Fatalf("got %t want %t", got, test.want)
			}
		})
	}
}

func TestBrowserRpaAuthorityIsServerResolvedBoundAndRevalidated(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	repo := newControlledMemoryRepository()
	binding := validControlledBinding()
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: "change_1", OrganizationID: "org_1", ProjectID: "project_1", Binding: binding, Action: ControlledActionCreateProjectAndPromotions, BudgetLimitMinor: 30000, Currency: "CNY", Status: ControlledChangeSetExecuting, Version: 3, CreatedBy: "operator", CreatedAt: now.Add(-time.Hour), UpdatedAt: now}
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	approval := RemoteWriteApproval{SchemaVersion: RemoteWriteApprovalSchemaV1, ID: "approval_1", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID, ControlledChangeSetID: change.ID, ControlledChangeSetHash: change.CanonicalHash, Binding: binding, Action: change.Action, Scope: "controlled_remote_write", BudgetLimitMinor: change.BudgetLimitMinor, Currency: change.Currency, ApprovedBy: "approver", ApprovedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	approval.ActionHash, _ = approval.ComputeActionHash()
	execution := ControlledExecution{ID: "execution_1", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID, ControlledChangeSetID: change.ID, RemoteWriteApprovalID: approval.ID, Status: "pending", Version: 1, CreatedBy: "operator", CreatedAt: now, UpdatedAt: now}
	repo.changes[repositoryKey(change.OrganizationID, change.ProjectID, change.ID)] = change
	repo.approvals[repositoryKey(change.OrganizationID, change.ProjectID, change.ID)] = approval
	repo.executions[repositoryKey(change.OrganizationID, change.ProjectID, execution.ID)] = execution
	provider := BrowserRpaAuthorityProvider{Repository: repo}
	resolved, err := provider.ResolveAuthority(context.Background(), change.OrganizationID, change.ProjectID, execution.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.BoundRunID != "" || resolved.Binding.ApprovalID != approval.ID || resolved.Binding.ApprovalActionHash != approval.ActionHash || resolved.Binding.WorkflowStepID != "submit-platform-configuration" || resolved.Binding.SkillID != binding.SkillID || resolved.Binding.PlanID != binding.PlanID || resolved.Binding.PlanVersion != binding.PlanVersion {
		t.Fatalf("resolved=%+v", resolved)
	}
	if err := provider.BindRun(context.Background(), resolved.Binding, "run_1", now); err != nil {
		t.Fatal(err)
	}
	if err := provider.VerifyAuthority(context.Background(), resolved.Binding, "run_1", now); err != nil {
		t.Fatal(err)
	}
	replayed, err := provider.ResolveAuthority(context.Background(), change.OrganizationID, change.ProjectID, execution.ID, now)
	if err != nil || replayed.BoundRunID != "run_1" {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	if err := provider.VerifyAuthority(context.Background(), resolved.Binding, "run_2", now); err != browserautomation.ErrInvalidContract {
		t.Fatalf("cross-run verification err=%v", err)
	}
	if _, err := provider.ResolveAuthority(context.Background(), change.OrganizationID, change.ProjectID, execution.ID, approval.ExpiresAt); err != browserautomation.ErrInvalidContract {
		t.Fatalf("expired approval err=%v", err)
	}
}

func TestPlanExecutionPrepareRetryAcceptsExpiredServerApproval(t *testing.T) {
	now := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	repo := newControlledMemoryRepository()
	binding := validControlledBinding()
	binding.AuthorityOrigin = "plan_execution"
	binding.PreflightCanonicalHash = testHash("2")
	binding.SelectionID = ""
	binding.ObservatoryRunID = ""
	binding.ObservatoryRunCanonicalHash = ""
	binding.OperatorFeedbackID = ""
	binding.OperatorFeedbackCanonicalHash = ""
	binding.OperatorFeedbackDisposition = ""
	binding.DecisionID = ""
	binding.DecisionCanonicalHash = ""
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: "change_plan_retry", OrganizationID: "org_1", ProjectID: "project_1", Binding: binding, Action: ControlledActionCreateProjectAndPromotions, BudgetLimitMinor: 30000, Currency: "CNY", Status: ControlledChangeSetExecuting, Version: 3, CreatedBy: "operator", CreatedAt: now.Add(-time.Hour), UpdatedAt: now}
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	approval := RemoteWriteApproval{SchemaVersion: RemoteWriteApprovalSchemaV1, ID: "approval_plan_retry", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID, ControlledChangeSetID: change.ID, ControlledChangeSetHash: change.CanonicalHash, Binding: binding, Action: change.Action, Scope: "controlled_remote_write", BudgetLimitMinor: change.BudgetLimitMinor, Currency: change.Currency, ApprovedBy: "operator", ApprovedAt: now.Add(-time.Hour), ExpiresAt: now.Add(-30 * time.Minute)}
	approval.ActionHash, _ = approval.ComputeActionHash()
	execution := ControlledExecution{ID: "execution_plan_retry", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID, ControlledChangeSetID: change.ID, RemoteWriteApprovalID: approval.ID, Status: "pending", Version: 2, CreatedBy: "operator", CreatedAt: now.Add(-time.Hour), UpdatedAt: now}
	repo.changes[repositoryKey(change.OrganizationID, change.ProjectID, change.ID)] = change
	repo.approvals[repositoryKey(change.OrganizationID, change.ProjectID, change.ID)] = approval
	repo.executions[repositoryKey(change.OrganizationID, change.ProjectID, execution.ID)] = execution

	provider := BrowserRpaAuthorityProvider{Repository: repo}
	resolved, err := provider.ResolveAuthority(context.Background(), change.OrganizationID, change.ProjectID, execution.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Binding.AuthorityOrigin != "plan_execution" || resolved.Binding.ApprovalID != approval.ID {
		t.Fatalf("resolved=%+v", resolved)
	}
}

func TestBrowserRpaAuthorityCreatesAndConfirmsAllStagedMappings(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	repo := newControlledMemoryRepository()
	binding := validControlledBinding()
	configuration := PlatformConfiguration{ConfigurationID: binding.ConfigurationID, Payload: PlatformConfigurationPayload{OceanEngine: &OceanEngineConfiguration{
		Project:    &OceanEngineProjectDraft{ProjectDraftID: "project-draft-1"},
		Promotions: []OceanEnginePromotionDraft{{PromotionDraftID: "promotion-draft-1"}, {PromotionDraftID: "promotion-draft-2"}},
	}}}
	repo.plans[repositoryKey("org_1", "project_1", binding.PlanID)] = DeliveryPlan{
		ID: binding.PlanID, OrganizationID: "org_1", ProjectID: "project_1",
		Versions: []DeliveryPlanVersion{{VersionNumber: binding.PlanVersion, PlatformConfiguration: &configuration}},
	}
	change := ControlledChangeSet{SchemaVersion: ControlledChangeSetSchemaV1, ID: "change_staged", OrganizationID: "org_1", ProjectID: "project_1", Binding: binding, Action: ControlledActionCreateProjectAndPromotions, BudgetLimitMinor: 30000, Currency: "CNY", Status: ControlledChangeSetExecuting, Version: 2, CreatedBy: "operator", CreatedAt: now, UpdatedAt: now}
	change.CanonicalHash, _ = change.ComputeCanonicalHash()
	approval := RemoteWriteApproval{SchemaVersion: RemoteWriteApprovalSchemaV1, ID: "approval_staged", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID, ControlledChangeSetID: change.ID, ControlledChangeSetHash: change.CanonicalHash, Binding: binding, Action: change.Action, Scope: "controlled_remote_write", BudgetLimitMinor: change.BudgetLimitMinor, Currency: change.Currency, ApprovedBy: "approver", ApprovedAt: now, ExpiresAt: now.Add(time.Hour)}
	approval.ActionHash, _ = approval.ComputeActionHash()
	execution := ControlledExecution{ID: "execution_staged", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID, ControlledChangeSetID: change.ID, RemoteWriteApprovalID: approval.ID, Status: "pending", Version: 1, CreatedBy: "operator", CreatedAt: now, UpdatedAt: now}
	repo.changes[repositoryKey(change.OrganizationID, change.ProjectID, change.ID)] = change
	repo.approvals[repositoryKey(change.OrganizationID, change.ProjectID, change.ID)] = approval
	repo.executions[repositoryKey(change.OrganizationID, change.ProjectID, execution.ID)] = execution
	staleProjectMapping := PlatformEntityMapping{
		SchemaVersion: PlatformEntityMappingV1, ID: "mapping_stale_project", OrganizationID: change.OrganizationID, ProjectID: change.ProjectID,
		AccountReferenceID: binding.AccountReferenceID, PlanID: binding.PlanID, ConfigurationID: "configuration_previous",
		BusinessExecutionID: "execution_previous", BrowserRpaRunID: "run_previous",
		InternalObjectKind: "project", InternalObjectID: "project-draft-1", PlatformObjectKind: "project",
		Status: PlatformEntityMappingPending, Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	repo.mappings[repositoryKey(change.OrganizationID, change.ProjectID, staleProjectMapping.ID)] = staleProjectMapping

	provider := BrowserRpaAuthorityProvider{Repository: repo}
	resolved, err := provider.ResolveAuthority(context.Background(), change.OrganizationID, change.ProjectID, execution.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.BindRun(context.Background(), resolved.Binding, "run_staged", now); err != nil {
		t.Fatal(err)
	}
	if len(repo.mappings) != 3 {
		t.Fatalf("staged mappings=%d want=3", len(repo.mappings))
	}
	recoveredProject, err := repo.GetPlatformEntityMappingByInternalObject(context.Background(), change.OrganizationID, change.ProjectID, binding.AccountReferenceID, "project", "project-draft-1")
	if err != nil || recoveredProject.BusinessExecutionID != execution.ID || recoveredProject.BrowserRpaRunID != "run_staged" || recoveredProject.ConfigurationID != binding.ConfigurationID {
		t.Fatalf("recovered project mapping=%#v err=%v", recoveredProject, err)
	}
	if _, err := provider.RecordCreatedObject(context.Background(), resolved.Binding, "run_staged", browserautomation.PreparedPage{
		InternalObjectKind: "project", InternalObjectID: "project-draft-1",
		Readback: map[string]string{"platform_object_id": "7677595885572784182", "reconciliation": "matched", "field_reconciliation_status": "not_checked"},
	}, "unchecked_result", "unchecked_list", now); err != browserautomation.ErrInvalidContract {
		t.Fatalf("unchecked fields writeback err=%v", err)
	}

	targets := []struct{ kind, internalID, platformID string }{
		{"project", "project-draft-1", "7677595885572784182"},
		{"promotion", "promotion-draft-1", "7683558668450021382"},
		{"promotion", "promotion-draft-2", "7683558668450021383"},
	}
	for index, target := range targets {
		mapping, getErr := repo.GetPlatformEntityMappingByInternalObject(context.Background(), change.OrganizationID, change.ProjectID, binding.AccountReferenceID, target.kind, target.internalID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		resultID, listID := fmt.Sprintf("result_%d", index), fmt.Sprintf("list_%d", index)
		repo.evidence[resultID] = validMappingEvidence(mapping, resultID, "step_"+resultID, index*10+2, browserautomation.TakeoverResultObserved, target.platformID, "pending_review")
		repo.evidence[listID] = validMappingEvidence(mapping, listID, "step_"+listID, index*10+3, browserautomation.TakeoverListConfirmed, target.platformID, "pending_review")
		complete, recordErr := provider.RecordCreatedObject(context.Background(), resolved.Binding, "run_staged", browserautomation.PreparedPage{InternalObjectKind: target.kind, InternalObjectID: target.internalID, Readback: map[string]string{"platform_object_id": target.platformID, "reconciliation": "matched"}}, resultID, listID, now)
		if recordErr != nil {
			t.Fatal(recordErr)
		}
		if complete != (index == len(targets)-1) {
			t.Fatalf("stage %d complete=%t", index, complete)
		}
	}
	completed, err := repo.GetControlledExecution(context.Background(), change.OrganizationID, change.ProjectID, execution.ID)
	if err != nil || completed.Status != "succeeded" {
		t.Fatalf("execution=%#v err=%v", completed, err)
	}
}

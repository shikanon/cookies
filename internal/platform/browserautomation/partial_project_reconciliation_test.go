package browserautomation

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPartialProjectRecoveryRequiresMatchingReadOnlyEvidence(t *testing.T) {
	for _, scenario := range []string{"matched", "drifted", "different_object", "write", "other_field", "no_click"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			now := time.Now()
			repo := NewMemoryRepository()
			provider := &stagedRecorderProvider{}
			counter := 0
			service := Service{Repository: repo, AuthorityProvider: provider, Now: func() time.Time { return now }, NewID: func(prefix string) (string, error) { counter++; return fmt.Sprintf("%s_%d", prefix, counter), nil }}
			run := validRun(now)
			run.State = RunPartial
			_, _, _ = repo.CreateRun(ctx, run)
			original := Evidence{ID: "original", RunID: run.ID, OrganizationID: run.OrganizationID, ProjectID: run.ProjectID, ObjectFingerprint: "project-draft-1", DiffKeys: []string{"project.optimization_target_reference"}, FieldReadback: map[string]string{"platform_object_id": "12345", "final_click_performed": "true", "field_reconciliation_status": "drifted"}}
			if scenario == "other_field" {
				original.DiffKeys = append(original.DiffKeys, "project.daily_budget")
			}
			if scenario == "no_click" {
				original.FieldReadback["final_click_performed"] = "false"
			}
			_ = repo.AppendEvidence(ctx, original)
			if _, err := service.TransitionRun(ctx, run.OrganizationID, run.ProjectID, run.ID, run.Version, RunEnvironmentCheck, ""); err == nil {
				t.Fatal("partial state resumed without verified evidence")
			}
			page := PreparedPage{InternalObjectKind: "project", InternalObjectID: "project-draft-1", Readback: map[string]string{"platform_object_id": "12345", "reconciliation": "matched", "field_reconciliation_status": "matched", "read_only_reconciliation": "true", "platform_write_performed": "false"}}
			if scenario == "drifted" {
				page.Readback["field_reconciliation_status"] = "drifted"
			}
			if scenario == "different_object" {
				page.Readback["platform_object_id"] = "67890"
			}
			if scenario == "write" {
				page.Readback["platform_write_performed"] = "true"
			}
			result, err := (Worker{Service: service}).ReconcileResultUnknown(ctx, run.OrganizationID, run.ProjectID, run.ID, page)
			if scenario == "matched" {
				if err != nil || result.State != RunEnvironmentCheck {
					t.Fatalf("run = %#v, error = %v", result, err)
				}
			} else if err == nil {
				t.Fatal("unsafe recovery accepted")
			}
			if len(repo.attempts) != 0 || len(provider.recorded) != 0 {
				t.Fatal("recovery wrote a controlled action or replaced a mapping")
			}
		})
	}
}

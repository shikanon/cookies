package main

import (
	"context"
	"fmt"

	"github.com/shikanon/cookies/internal/platform/ids"
	"github.com/shikanon/cookies/internal/platform/project"
	"github.com/shikanon/cookies/internal/systems/creative"
)

type projectAuditAppender interface {
	AppendAuditEvent(context.Context, project.AuditEvent) error
}

type productionRetryAuditAdapter struct{ store projectAuditAppender }

func (a productionRetryAuditAdapter) AppendProductionRetryAudit(ctx context.Context, event creative.ProductionRetryAuditEvent) error {
	id, err := ids.New("auditevent")
	if err != nil {
		return err
	}
	return a.store.AppendAuditEvent(ctx, project.AuditEvent{
		ID: id, OrganizationID: event.OrganizationID, ProjectID: event.ProjectID,
		Actor: fmt.Sprintf("%s:%s", event.Actor.Kind, event.Actor.ID), Action: event.Action,
		EntityType: project.AuditEntityGenerationJob, EntityID: event.NewRun.Key(),
		Metadata: map[string]any{
			"previous_run": map[string]any{"source": event.PreviousRun.Source, "id": event.PreviousRun.ID},
			"new_run":      map[string]any{"source": event.NewRun.Source, "id": event.NewRun.ID},
		},
		CreatedAt: event.OccurredAt,
	})
}

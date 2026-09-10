package delivery

import (
	"context"
	"fmt"
	"github.com/shikanon/cookies/internal/platform/contract"
)

type FieldAvailability struct {
	State  string `json:"state"`
	Reason string `json:"reason"`
	Source string `json:"source"`
}

type PlatformFieldObservation struct {
	Key      string            `json:"key"`
	Platform FieldAvailability `json:"platform"`
}

type FieldCapabilityRequest struct {
	AccountID string `json:"account_id"`
	ObjectID  string `json:"object_id"`
	Kind      string `json:"kind"`
}

type FieldCapabilitySnapshot struct {
	AccountID     string                     `json:"account_id"`
	ObjectID      string                     `json:"object_id"`
	Kind          string                     `json:"kind"`
	ObservedAt    string                     `json:"observed_at"`
	ObjectCanEdit *bool                      `json:"object_can_edit,omitempty"`
	Fields        []ObjectFieldPolicy        `json:"fields"`
	Observations  []PlatformFieldObservation `json:"observations"`
}

type FieldCapabilityReader interface {
	ReadFieldCapabilities(context.Context, FieldCapabilityRequest) (FieldCapabilitySnapshot, error)
}

func (s Service) ReadObjectFieldCapabilities(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, mappingID string) (FieldCapabilitySnapshot, error) {
	mapping, err := s.GetPlatformEntityMapping(ctx, actor, projectID, mappingID)
	if err != nil {
		return FieldCapabilitySnapshot{}, err
	}
	if mapping.Status != PlatformEntityMappingConfirmed {
		return FieldCapabilitySnapshot{}, ErrInvalidState
	}
	if s.FieldCapabilities == nil || s.ExternalAccountIDs == nil {
		return FieldCapabilitySnapshot{}, ErrUnsupportedConfigurationWorkflow
	}
	plan, err := s.GetPlan(ctx, actor, projectID, mapping.PlanID)
	if err != nil {
		return FieldCapabilitySnapshot{}, err
	}
	preview, err := s.planObjectPreview(ctx, actor, projectID, plan)
	if err != nil {
		return FieldCapabilitySnapshot{}, err
	}
	var fields []ObjectFieldPolicy
	for _, object := range preview.Objects {
		if object.MappingID == mapping.ID {
			fields = object.Fields
		}
	}
	if fields == nil {
		return FieldCapabilitySnapshot{}, ErrInvalidState
	}
	accountID, err := s.ExternalAccountIDs.ResolveExternalAccountID(ctx, string(actor.OrganizationID), string(projectID), plan.CurrentVersion.PlatformConfiguration.Payload.OceanEngine.Project.AccountReference.ID)
	if err != nil {
		return FieldCapabilitySnapshot{}, err
	}
	if mapping.AccountReferenceID != accountID && mapping.AccountReferenceID != plan.CurrentVersion.PlatformConfiguration.Payload.OceanEngine.Project.AccountReference.ID {
		return FieldCapabilitySnapshot{}, ErrInvalidState
	}
	request := FieldCapabilityRequest{AccountID: accountID, ObjectID: mapping.PlatformObjectID, Kind: mapping.InternalObjectKind}
	snapshot, err := s.FieldCapabilities.ReadFieldCapabilities(ctx, request)
	if err != nil {
		return FieldCapabilitySnapshot{}, err
	}
	if snapshot.AccountID != request.AccountID || snapshot.ObjectID != request.ObjectID || snapshot.Kind != request.Kind || snapshot.ObservedAt == "" {
		return FieldCapabilitySnapshot{}, fmt.Errorf("%w: field observation identity mismatch", ErrInvalidState)
	}

	for i := range fields {
		for _, observation := range snapshot.Observations {
			if observation.Key == fields[i].Key {
				fields[i].Platform = observation.Platform
			}
		}
	}
	snapshot.Fields = fields
	return snapshot, nil
}

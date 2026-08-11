package creative

import (
	"context"
	"errors"
	"fmt"
	"time"

	platformassets "github.com/shikanon/cookies/internal/platform/assets"
	"github.com/shikanon/cookies/internal/platform/contract"
)

var ErrOperationVersionConflict = errors.New("editing operation base version conflicts with the current timeline")

type EditOperationAudit struct {
	OrganizationID        contract.OrganizationID
	ProjectID             contract.ProjectID
	EditTaskID            string
	Batch                 EditOperationBatch
	ResultTimelineVersion int64
	InverseOperations     []EditOperation
	ResultContentHash     string
	ChangeSummary         string
	CreatedAt             time.Time
}

type EditOperationRepository interface {
	AppendEditOperations(context.Context, EditTask, int64, TimelineVersion, EditOperationAudit) (EditTask, error)
}

type ApplyEditOperationsRequest struct {
	Batch EditOperationBatch `json:"batch"`
}

func (s Service) ApplyEditOperations(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, taskID string, request ApplyEditOperationsRequest) (EditTask, error) {
	if err := s.validateEditingDependencies(ctx, actor, projectID, true); err != nil {
		return EditTask{}, err
	}
	repository, ok := s.EditTasks.(EditOperationRepository)
	if !ok {
		return EditTask{}, fmt.Errorf("editing operation repository is not configured")
	}
	task, err := s.EditTasks.GetEditTask(ctx, actor.OrganizationID, projectID, taskID)
	if err != nil {
		return EditTask{}, err
	}
	if task.CurrentTimeline == nil || task.CurrentTimeline.Version != request.Batch.BaseTimelineVersion {
		return EditTask{}, ErrOperationVersionConflict
	}
	if request.Batch.Actor == "" {
		request.Batch.Actor = actor.Principal.ID
		for index := range request.Batch.Operations {
			request.Batch.Operations[index].Actor = actor.Principal.ID
			request.Batch.Operations[index].BaseTimelineVersion = request.Batch.BaseTimelineVersion
		}
	}
	if request.Batch.Actor != actor.Principal.ID {
		return EditTask{}, fmt.Errorf("operation actor must match the authenticated principal")
	}
	document := EditingDocument{}
	if task.CurrentTimeline.Schema() == EditingTimelineSchemaV2 {
		copy := *task.CurrentTimeline.TimelineV2
		ensureC3VisualTracks(&copy)
		ensureC4CaptionTrack(&copy)
		ensureC5AudioTracks(&copy)
		document.V2 = &copy
	} else {
		document.V1 = &task.CurrentTimeline.Timeline
		document, err = DefaultEditingCodecRegistry().MigrateToV2(document)
		if err != nil {
			return EditTask{}, err
		}
	}
	result, err := ApplyEditOperations(document, request.Batch)
	if err != nil {
		return EditTask{}, err
	}
	if err := s.validateTimelineV2Assets(ctx, actor, projectID, *result.Document.V2, platformassets.AssetUseTimelineSave); err != nil {
		return EditTask{}, err
	}
	hash, err := editingDocumentHash(result.Document)
	if err != nil {
		return EditTask{}, err
	}
	now := s.now()
	version := TimelineVersion{Version: task.CurrentTimeline.Version + 1, SchemaVersion: EditingTimelineSchemaV2, TimelineV2: result.Document.V2, ParentVersion: task.CurrentTimeline.Version, ChangeSummary: result.ChangeSummary, OperationBatchID: request.Batch.BatchID, CompilerCompatibility: "editing-v2-audio-c5", ContentHash: hash, CreatedBy: actor.Principal.ID, CreatedAt: now}
	audit := EditOperationAudit{OrganizationID: actor.OrganizationID, ProjectID: projectID, EditTaskID: task.ID, Batch: request.Batch, ResultTimelineVersion: version.Version, InverseOperations: result.InverseOperations, ResultContentHash: hash, ChangeSummary: result.ChangeSummary, CreatedAt: now}
	updated, err := repository.AppendEditOperations(ctx, task, request.Batch.BaseTimelineVersion, version, audit)
	if errors.Is(err, ErrVersionConflict) {
		return EditTask{}, ErrOperationVersionConflict
	}
	return updated, err
}

func ensureC5AudioTracks(timeline *EditingTimelineV2) {
	roles := []string{"voiceover", "music", "sfx"}
	for _, role := range roles {
		found := false
		for _, track := range timeline.Tracks {
			if track.Kind == "audio" && track.Role == role {
				found = true
				break
			}
		}
		if found {
			continue
		}
		id := "audio-" + role
		for _, track := range timeline.Tracks {
			if track.ID == id {
				id += "-c5"
				break
			}
		}
		timeline.Tracks = append(timeline.Tracks, EditingTrackV2{ID: id, Kind: "audio", Role: role, Clips: []EditingClipV2{}})
	}
}

func ensureC4CaptionTrack(timeline *EditingTimelineV2) {
	for _, track := range timeline.Tracks {
		if track.Kind == "caption" {
			return
		}
	}
	id := "captions-main"
	for _, track := range timeline.Tracks {
		if track.ID == id {
			id = "captions-main-c4"
			break
		}
	}
	timeline.Tracks = append(timeline.Tracks, EditingTrackV2{ID: id, Kind: "caption", Language: "zh-CN", Clips: []EditingClipV2{}})
}

func ensureC3VisualTracks(timeline *EditingTimelineV2) {
	usedIDs := map[string]bool{}
	usedZ := map[int]bool{}
	visuals := 0
	for _, track := range timeline.Tracks {
		usedIDs[track.ID] = true
		if track.Kind == "visual" {
			visuals++
			usedZ[track.ZIndex] = true
		}
	}
	for z := 0; visuals < 3 && z <= 2; z++ {
		if usedZ[z] {
			continue
		}
		id := fmt.Sprintf("visual-overlay-%d", z)
		for usedIDs[id] {
			id += "-c3"
		}
		timeline.Tracks = append(timeline.Tracks, EditingTrackV2{ID: id, Kind: "visual", Role: "overlay", ZIndex: z, Clips: []EditingClipV2{}})
		usedIDs[id], usedZ[z], visuals = true, true, visuals+1
	}
}

func (s Service) validateTimelineV2Assets(ctx context.Context, actor contract.ActorContext, projectID contract.ProjectID, timeline EditingTimelineV2, purpose platformassets.AssetUsePurpose) error {
	if err := timeline.Validate(); err != nil {
		return err
	}
	for _, track := range timeline.Tracks {
		for _, clip := range track.Clips {
			if clip.AssetRef == nil {
				continue
			}
			asset, err := s.Assets.ReadForCreative(ctx, actor, projectID, *clip.AssetRef)
			if err != nil {
				return err
			}
			if !asset.Ready || asset.Ref != *clip.AssetRef {
				return fmt.Errorf("editing clip %s references an unavailable asset", clip.ID)
			}
			if clip.Kind == "video" && asset.Kind != contract.AssetVideo || clip.Kind == "image" && asset.Kind != contract.AssetImage || clip.Kind == "audio" && asset.Kind != contract.AssetAudio {
				return fmt.Errorf("editing clip %s asset kind does not match", clip.ID)
			}
			if clip.Source != nil && clip.Source.OutUS > asset.DurationMS*1000 {
				return fmt.Errorf("editing clip %s source range exceeds asset duration", clip.ID)
			}
			if s.AssetUses != nil {
				if _, err := s.AssetUses.Authorize(ctx, platformassets.AssetUseRequest{OrganizationID: actor.OrganizationID, ProjectID: projectID, AssetRef: *clip.AssetRef, Purpose: purpose}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

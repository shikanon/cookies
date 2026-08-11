package assets

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shikanon/cookies/internal/platform/contract"
	"github.com/shikanon/cookies/internal/platform/ids"
)

type DerivativeProfile string

const (
	DerivativeVideoProxy DerivativeProfile = "video_proxy_v1"
	DerivativePoster     DerivativeProfile = "poster_v1"
	DerivativeWaveform   DerivativeProfile = "waveform_v1"
	DerivativeFontWeb    DerivativeProfile = "font_woff2_v1"
)

type DerivativeStatus string

const (
	DerivativeQueued  DerivativeStatus = "queued"
	DerivativeRunning DerivativeStatus = "running"
	DerivativeReady   DerivativeStatus = "ready"
	DerivativeFailed  DerivativeStatus = "failed"
)

type ProcessingStatus string

const (
	ProcessingQueued    ProcessingStatus = "queued"
	ProcessingRunning   ProcessingStatus = "running"
	ProcessingSucceeded ProcessingStatus = "succeeded"
	ProcessingFailed    ProcessingStatus = "failed"
)

type AssetDerivative struct {
	ID             string                    `json:"id"`
	OrganizationID contract.OrganizationID   `json:"organization_id"`
	ProjectID      contract.ProjectID        `json:"project_id"`
	Source         contract.AssetVersionRef  `json:"source"`
	Profile        DerivativeProfile         `json:"profile"`
	Status         DerivativeStatus          `json:"status"`
	Output         *contract.AssetVersionRef `json:"output,omitempty"`
	ErrorCode      string                    `json:"error_code,omitempty"`
	CreatedAt      time.Time                 `json:"created_at"`
	UpdatedAt      time.Time                 `json:"updated_at"`
}

type ProcessingJob struct {
	ID             string                  `json:"id"`
	OrganizationID contract.OrganizationID `json:"organization_id"`
	ProjectID      contract.ProjectID      `json:"project_id"`
	DerivativeID   string                  `json:"derivative_id"`
	Status         ProcessingStatus        `json:"status"`
	Attempt        int                     `json:"attempt"`
	ErrorCode      string                  `json:"error_code,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

type EnsureDerivativeRequest struct {
	OrganizationID contract.OrganizationID
	ProjectID      contract.ProjectID
	AssetRef       contract.AssetVersionRef
	Profile        DerivativeProfile
}

type DerivativeRepository interface {
	EnsureDerivative(context.Context, AssetDerivative, ProcessingJob) (AssetDerivative, ProcessingJob, bool, error)
	FailDerivativeScheduling(context.Context, string, string, time.Time) (AssetDerivative, ProcessingJob, error)
	RetryDerivative(context.Context, contract.OrganizationID, contract.ProjectID, string, string, time.Time) (AssetDerivative, ProcessingJob, error)
}

type DerivativeScheduler interface {
	ScheduleAssetDerivative(context.Context, ProcessingJob) error
}

type DerivativeService struct {
	Repository DerivativeRepository
	Scheduler  DerivativeScheduler
	NewID      ids.Generator
	Now        func() time.Time
}

func (s DerivativeService) EnsureDerivative(ctx context.Context, request EnsureDerivativeRequest) (AssetDerivative, ProcessingJob, bool, error) {
	if s.Repository == nil || s.Scheduler == nil {
		return AssetDerivative{}, ProcessingJob{}, false, fmt.Errorf("derivative repository and scheduler are required")
	}
	if request.OrganizationID == "" || request.ProjectID == "" || !validDerivativeProfile(request.Profile) {
		return AssetDerivative{}, ProcessingJob{}, false, fmt.Errorf("derivative scope and supported profile are required")
	}
	if err := request.AssetRef.Validate(); err != nil {
		return AssetDerivative{}, ProcessingJob{}, false, err
	}
	newID := s.NewID
	if newID == nil {
		newID = ids.New
	}
	derivativeID, err := newID("derivative")
	if err != nil {
		return AssetDerivative{}, ProcessingJob{}, false, err
	}
	jobID, err := newID("assetjob")
	if err != nil {
		return AssetDerivative{}, ProcessingJob{}, false, err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now()
	}
	value := AssetDerivative{ID: derivativeID, OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, Source: request.AssetRef, Profile: request.Profile, Status: DerivativeQueued, CreatedAt: now, UpdatedAt: now}
	job := ProcessingJob{ID: jobID, OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, DerivativeID: derivativeID, Status: ProcessingQueued, Attempt: 1, CreatedAt: now, UpdatedAt: now}
	value, job, duplicate, err := s.Repository.EnsureDerivative(ctx, value, job)
	if err != nil || duplicate {
		return value, job, duplicate, err
	}
	if err := s.Scheduler.ScheduleAssetDerivative(ctx, job); err != nil {
		value, job, markErr := s.Repository.FailDerivativeScheduling(ctx, value.ID, "DERIVATIVE_QUEUE_FAILED", now)
		if markErr != nil {
			return value, job, false, fmt.Errorf("schedule derivative: %v; mark failed: %w", err, markErr)
		}
		return value, job, false, fmt.Errorf("schedule derivative: %w", err)
	}
	return value, job, false, nil
}

func (s DerivativeService) RetryDerivative(ctx context.Context, org contract.OrganizationID, project contract.ProjectID, id string) (AssetDerivative, ProcessingJob, error) {
	if s.Repository == nil || s.Scheduler == nil || strings.TrimSpace(id) == "" {
		return AssetDerivative{}, ProcessingJob{}, fmt.Errorf("derivative repository, scheduler and id are required")
	}
	newID := s.NewID
	if newID == nil {
		newID = ids.New
	}
	jobID, err := newID("assetjob")
	if err != nil {
		return AssetDerivative{}, ProcessingJob{}, err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now()
	}
	value, job, err := s.Repository.RetryDerivative(ctx, org, project, id, jobID, now)
	if err != nil {
		return value, job, err
	}
	if err := s.Scheduler.ScheduleAssetDerivative(ctx, job); err != nil {
		value, job, _ = s.Repository.FailDerivativeScheduling(ctx, value.ID, "DERIVATIVE_QUEUE_FAILED", now)
		return value, job, fmt.Errorf("schedule derivative retry: %w", err)
	}
	return value, job, nil
}

func validDerivativeProfile(profile DerivativeProfile) bool {
	switch profile {
	case DerivativeVideoProxy, DerivativePoster, DerivativeWaveform, DerivativeFontWeb:
		return true
	}
	return false
}

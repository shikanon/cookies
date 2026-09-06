package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/shikanon/cookies/internal/platform/browserautomation"
	"github.com/shikanon/cookies/internal/systems/delivery"
)

type deliveryBrowserRpaLauncher struct {
	service         browserautomation.Service
	executionDriver browserautomation.ExecutionDriver
}

func (l deliveryBrowserRpaLauncher) ReconcileBrowserRpaRun(ctx context.Context, request delivery.BrowserRpaLaunchRequest, runID string) error {
	now := time.Now().UTC()
	if l.service.Now != nil {
		now = l.service.Now()
	}
	resolution, err := l.service.AuthorityProvider.ResolveAuthority(ctx, request.OrganizationID, request.ProjectID, request.BusinessExecutionID, now)
	if err != nil {
		return fmt.Errorf("resolve Browser RPA retry authority: %w", err)
	}
	if resolution.BoundRunID != runID {
		return fmt.Errorf("resolve Browser RPA retry authority: bound run mismatch")
	}
	existing, err := l.service.Repository.GetRun(ctx, request.OrganizationID, request.ProjectID, runID)
	if err != nil {
		return fmt.Errorf("load Browser RPA retry run: %w", err)
	}
	if request.ExecutionDriver != "" && existing.EffectiveExecutionDriver() != request.ExecutionDriver {
		return fmt.Errorf("resolve Browser RPA retry authority: execution driver mismatch")
	}
	if err := l.service.AuthorityProvider.BindRun(ctx, resolution.Binding, runID, now); err != nil {
		return fmt.Errorf("reconcile Browser RPA run: %w", err)
	}
	return nil
}

func (l deliveryBrowserRpaLauncher) LaunchBrowserRpaRun(ctx context.Context, request delivery.BrowserRpaLaunchRequest) (delivery.BrowserRpaLaunchResult, error) {
	driver := l.executionDriver
	if request.ExecutionDriver != "" {
		driver = request.ExecutionDriver
	}
	if driver == "" {
		driver = browserautomation.ExecutionDriverPlaywrightEdgeV3
	}
	key := launcherKey(string(request.OrganizationID), string(request.ProjectID), request.AccountID)
	environmentID := "rpaenv_" + key
	profileID := "rpaprofile_" + key
	policyID := "rpapolicy_" + key
	environmentMode, browserVersion, region := "local_visible", "external-edge", "local"
	if driver == browserautomation.ExecutionDriverOceanEngineWebAPI {
		environmentID = "apienv_" + key
		profileID = "apisession_" + key
		policyID = "apipolicy_" + key
		environmentMode, browserVersion, region = "remote_api", "secsdk-1.2.22", "remote"
	}
	environment := browserautomation.ExecutionEnvironment{ID: environmentID, Platform: browserautomation.PlatformOceanEngine, AccountID: request.AccountID, Mode: environmentMode, BrowserVersion: browserVersion, Region: region, Healthy: true}
	if _, err := l.service.Repository.GetEnvironment(ctx, request.OrganizationID, request.ProjectID, environmentID); err != nil {
		if _, createErr := l.service.RegisterEnvironment(ctx, request.OrganizationID, request.ProjectID, environment); createErr != nil {
			return delivery.BrowserRpaLaunchResult{}, fmt.Errorf("register Browser RPA environment: %w", createErr)
		}
	}
	profile := browserautomation.BrowserProfile{ID: profileID, EnvironmentID: environmentID, Platform: browserautomation.PlatformOceanEngine, AccountID: request.AccountID, State: "ready"}
	if _, err := l.service.Repository.GetBrowserProfile(ctx, request.OrganizationID, request.ProjectID, profileID); err != nil {
		if _, createErr := l.service.RegisterBrowserProfile(ctx, request.OrganizationID, request.ProjectID, profile); createErr != nil {
			return delivery.BrowserRpaLaunchResult{}, fmt.Errorf("register Browser RPA profile: %w", createErr)
		}
	}
	allowedProjects := []string{"unbound_project"}
	if request.ParentProjectID != "" {
		allowedProjects = []string{request.ParentProjectID}
	}
	policy := browserautomation.SitePolicy{ID: policyID, Platform: browserautomation.PlatformOceanEngine, AccountID: request.AccountID, AllowedProtocols: []string{"https"}, AllowedHosts: []string{"ad.oceanengine.com"}, AllowedPageKinds: []string{"project_create", "project_edit", "promotion_create", "promotion_edit"}, AllowedPlatformProjects: allowedProjects}
	if _, err := l.service.Repository.GetSitePolicy(ctx, request.OrganizationID, request.ProjectID, policyID); err != nil {
		if _, createErr := l.service.RegisterSitePolicy(ctx, request.OrganizationID, request.ProjectID, policy); createErr != nil {
			return delivery.BrowserRpaLaunchResult{}, fmt.Errorf("register Browser RPA site policy: %w", createErr)
		}
	}
	run, _, err := l.service.CreateBoundRun(ctx, browserautomation.CreateBoundRunRequest{OrganizationID: request.OrganizationID, ProjectID: request.ProjectID, Platform: browserautomation.PlatformOceanEngine, AccountID: request.AccountID, ExecutionDriver: driver, ExecutionID: request.BusinessExecutionID, EnvironmentID: environmentID, ProfileID: profileID, PolicyID: policyID, IdempotencyKey: request.IdempotencyKey, CreatedBy: request.CreatedBy})
	if err != nil {
		return delivery.BrowserRpaLaunchResult{}, fmt.Errorf("create Browser RPA run: %w", err)
	}
	return delivery.BrowserRpaLaunchResult{RunID: run.ID}, nil
}

func launcherKey(values ...string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte(value))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))[:24]
}
